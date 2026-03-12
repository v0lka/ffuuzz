package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ffuuzz/internal/db"
	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/model"
)

type importRequest struct {
	Sessions []model.RecordingSession `json:"sessions"`
}

type importResult struct {
	Imported          int      `json:"imported"`
	Skipped           int      `json:"skipped"`
	Failed            int      `json:"failed"`
	Total             int      `json:"total"`
	SessionIDs        []string `json:"session_ids"`
	SkippedSessionIDs []string `json:"skipped_session_ids"`
	Errors            []string `json:"errors,omitempty"`
}

func (s *Server) importRecordings(c *gin.Context) {
	// #4: Validate Content-Type
	ct := c.ContentType()
	if ct != "" && !strings.Contains(ct, "application/json") {
		errorResponse(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return
	}

	// #4: Check body size (10MB limit)
	const maxImportBytes = 10 * 1024 * 1024
	if c.Request.ContentLength > maxImportBytes {
		errorResponse(c, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body exceeds 10MB limit")
		return
	}

	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", "invalid request body: "+err.Error())
		return
	}

	if len(req.Sessions) == 0 {
		errorResponse(c, http.StatusBadRequest, "EMPTY_SESSIONS", "sessions array is empty")
		return
	}

	// Check for duplicate IDs within the request
	seen := make(map[string]bool, len(req.Sessions))
	for _, sess := range req.Sessions {
		if seen[sess.ID] {
			errorResponse(c, http.StatusConflict, "DUPLICATE_ID", "duplicate session ID in request: "+sess.ID)
			return
		}
		seen[sess.ID] = true
	}

	result := importResult{
		SessionIDs:        make([]string, 0),
		SkippedSessionIDs: make([]string, 0),
	}

	for i := range req.Sessions {
		// Normalise the target path so imported recordings use the same
		// endpoint patterns as live recordings from the proxy.
		req.Sessions[i].Target.Path = endpoint.NormalizePath(req.Sessions[i].Target.Path)

		inserted, err := s.recordings.Upsert(c.Request.Context(), req.Sessions[i])
		if err != nil {
			s.logger.Error().Err(err).Str("recording_id", req.Sessions[i].ID).Msg("import session failed")
			result.Failed++
			result.Errors = append(result.Errors, "session "+req.Sessions[i].ID+": import failed")
			continue
		}
		if inserted {
			result.Imported++
			result.SessionIDs = append(result.SessionIDs, req.Sessions[i].ID)
			metrics.CorpusSize.Inc()
		} else {
			result.Skipped++
			result.SkippedSessionIDs = append(result.SkippedSessionIDs, req.Sessions[i].ID)
		}
	}

	result.Total = len(req.Sessions)
	c.JSON(http.StatusCreated, result)
}

func (s *Server) listRecordings(c *gin.Context) {
	limit, offset := parsePagination(c)
	host := c.Query("host")
	pathPrefix := c.Query("path_prefix")

	sessions, err := s.recordings.List(c.Request.Context(), limit, offset, host, pathPrefix)
	if err != nil {
		s.internalError(c, "LIST_FAILED", err)
		return
	}

	if len(sessions) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, sessions)
}

type exportResponse struct {
	Sessions []model.RecordingSession `json:"sessions"`
}

func (s *Server) exportRecordings(c *gin.Context) {
	host := c.Query("host")
	pathPrefix := c.Query("path_prefix")

	sessions, err := s.recordings.ListAll(c.Request.Context(), host, pathPrefix)
	if err != nil {
		s.internalError(c, "EXPORT_FAILED", err)
		return
	}

	c.Header("Content-Disposition", `attachment; filename="recordings-export.json"`)
	c.JSON(http.StatusOK, exportResponse{Sessions: sessions})
}

// treePathNode represents a node in the path hierarchy for the tree view.
type treePathNode struct {
	Segment        string         `json:"segment"`
	FullPath       string         `json:"full_path"`
	RecordingCount int            `json:"recording_count"`
	Children       []treePathNode `json:"children"`
}

// treeOrigin represents a top-level origin in the tree view.
type treeOrigin struct {
	Origin         string         `json:"origin"`
	Scheme         string         `json:"scheme"`
	Host           string         `json:"host"`
	Port           int            `json:"port"`
	RecordingCount int            `json:"recording_count"`
	Paths          []treePathNode `json:"paths"`
}

func (s *Server) getRecordingsTree(c *gin.Context) {
	entries, err := s.recordings.GetTree(c.Request.Context())
	if err != nil {
		s.internalError(c, "TREE_FAILED", err)
		return
	}

	tree := buildTree(entries)
	c.JSON(http.StatusOK, tree)
}

// buildTree transforms flat TreeEntry rows into a hierarchical tree structure.
func buildTree(entries []model.TreeEntry) []treeOrigin {
	// Group entries by origin (scheme+host+port)
	type originKey struct {
		Scheme string
		Host   string
		Port   int
	}
	originMap := make(map[originKey][]model.TreeEntry)
	var originOrder []originKey

	for _, e := range entries {
		key := originKey{Scheme: e.Scheme, Host: e.Host, Port: e.Port}
		if _, exists := originMap[key]; !exists {
			originOrder = append(originOrder, key)
		}
		originMap[key] = append(originMap[key], e)
	}

	sort.Slice(originOrder, func(i, j int) bool {
		a, b := originOrder[i], originOrder[j]
		if a.Scheme != b.Scheme {
			return a.Scheme < b.Scheme
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Port < b.Port
	})

	var result []treeOrigin
	for _, key := range originOrder {
		pathEntries := originMap[key]
		origin := treeOrigin{
			Origin: fmt.Sprintf("%s://%s:%d", key.Scheme, key.Host, key.Port),
			Scheme: key.Scheme,
			Host:   key.Host,
			Port:   key.Port,
		}

		// Build path trie
		root := &trieNode{children: make(map[string]*trieNode)}
		for _, pe := range pathEntries {
			origin.RecordingCount += pe.Count
			segments := splitPath(pe.Path)
			root.insert(segments, pe.Path, pe.Count)
		}

		origin.Paths = root.toTreePathNodes("")
		result = append(result, origin)
	}

	return result
}

// trieNode is used to build the path hierarchy.
type trieNode struct {
	children map[string]*trieNode
	count    int
	fullPath string // set for leaf paths that exist as recordings
}

func (n *trieNode) insert(segments []string, fullPath string, count int) {
	current := n
	for _, seg := range segments {
		if current.children[seg] == nil {
			current.children[seg] = &trieNode{children: make(map[string]*trieNode)}
		}
		current = current.children[seg]
	}
	current.count += count
	current.fullPath = fullPath
}

func (n *trieNode) toTreePathNodes(parentPath string) []treePathNode {
	var nodes []treePathNode
	// Sort children for deterministic output
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, seg := range keys {
		child := n.children[seg]
		fullPath := strings.TrimRight(parentPath, "/") + "/" + seg
		if child.fullPath != "" {
			fullPath = child.fullPath
		}
		node := treePathNode{
			Segment:        seg,
			FullPath:       fullPath,
			RecordingCount: child.count,
			Children:       child.toTreePathNodes(fullPath),
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// splitPath splits a URL path into segments, ignoring empty segments.
func splitPath(path string) []string {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

func (s *Server) deleteRecordingsByPrefix(c *gin.Context) {
	scheme := c.Query("scheme")
	host := c.Query("host")
	portStr := c.Query("port")
	pathPrefix := c.Query("path_prefix")

	if scheme == "" || host == "" || portStr == "" {
		errorResponse(c, http.StatusBadRequest, "MISSING_PARAMS", "scheme, host, and port are required")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_PORT", "port must be an integer")
		return
	}

	n, err := s.recordings.DeleteByPrefix(c.Request.Context(), scheme, host, port, pathPrefix)
	if err != nil {
		if errors.Is(err, db.ErrRecordingsInUse) {
			errorResponse(c, http.StatusConflict, "IN_USE", "some recordings are used by active campaigns")
			return
		}
		s.internalError(c, "DELETE_PREFIX_FAILED", err)
		return
	}

	for i := int64(0); i < n; i++ {
		metrics.CorpusSize.Dec()
	}

	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

func (s *Server) getRecording(c *gin.Context) {
	id := c.Param("id")
	includeEntries := c.DefaultQuery("include_entries", "false") == "true"
	maxBodyBytes, _ := strconv.Atoi(c.DefaultQuery("max_body_bytes", "0"))

	sess, err := s.recordings.GetByID(c.Request.Context(), id, includeEntries, maxBodyBytes)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if sess == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "recording not found")
		return
	}

	c.JSON(http.StatusOK, sess)
}

func (s *Server) deleteRecording(c *gin.Context) {
	id := c.Param("id")

	used, err := s.recordings.IsUsedByActiveCampaign(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "CHECK_FAILED", err)
		return
	}
	if used {
		errorResponse(c, http.StatusConflict, "IN_USE", "recording is used by an active campaign")
		return
	}

	deleted, err := s.recordings.Delete(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "DELETE_FAILED", err)
		return
	}
	if !deleted {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "recording not found")
		return
	}

	metrics.CorpusSize.Dec()
	c.Status(http.StatusNoContent)
}
