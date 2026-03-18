package mutate

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"math/rand"
	"strings"

	"ffuuzz/internal/model"
)

// JSONMutator applies JSON-aware structural mutations to request bodies
// that have a JSON Content-Type. Falls back to primitive byte mutations
// when the body is not valid JSON.
type JSONMutator struct {
	MaxBodyLen int
}

func (m *JSONMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	// Check if Content-Type indicates JSON
	ct := ""
	for _, v := range ex.Request.Headers["Content-Type"] {
		ct = v
		break
	}
	if ct == "" {
		for _, v := range ex.Request.Headers["content-type"] {
			ct = v
			break
		}
	}
	isJSON := strings.Contains(ct, "json")

	bodyBytes, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil || len(bodyBytes) == 0 {
		return MutationResult{Exchange: ex, Operators: []string{"json:noop"}}
	}

	// Try to parse as JSON regardless of Content-Type
	var data interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil || !isJSON {
		// Fallback to primitive byte mutation
		p := &PrimitiveMutator{}
		return p.Mutate(ex, rng, intensity)
	}

	op := rng.Intn(6)
	var opName string

	switch op {
	case 0:
		opName = "json:type_substitute"
		data = m.typeSubstitute(data, rng)
	case 1:
		opName = "json:object_key"
		data = m.objectKeyMutation(data, rng)
	case 2:
		opName = "json:array_mutation"
		data = m.arrayMutation(data, rng)
	case 3:
		opName = "json:boundary_values"
		data = m.boundaryValues(data, rng)
	case 4:
		opName = "json:depth_stress"
		data = m.depthStress(data, rng)
	case 5:
		opName = "json:string_mutation"
		data = m.stringMutation(data, rng)
	}

	mutated, err := json.Marshal(data)
	if err != nil {
		return MutationResult{Exchange: ex, Operators: []string{"json:marshal_error"}}
	}

	ex.Request.BodyB64 = base64.StdEncoding.EncodeToString(mutated)
	return MutationResult{Exchange: ex, Operators: []string{opName}}
}

func (m *JSONMutator) typeSubstitute(data interface{}, rng *rand.Rand) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			v[key] = m.substituteType(v[key], rng)
		}
		return v
	case []interface{}:
		if len(v) > 0 {
			idx := rng.Intn(len(v))
			v[idx] = m.substituteType(v[idx], rng)
		}
		return v
	default:
		return m.substituteType(data, rng)
	}
}

func (m *JSONMutator) substituteType(val interface{}, rng *rand.Rand) interface{} {
	switch rng.Intn(7) {
	case 0:
		return nil
	case 1:
		return true
	case 2:
		return false
	case 3:
		return rng.Float64() * 1000
	case 4:
		return randomString(rng, 10)
	case 5:
		return []interface{}{1, "a", nil}
	case 6:
		return map[string]interface{}{"fuzz": true}
	}
	return val
}

func (m *JSONMutator) objectKeyMutation(data interface{}, rng *rand.Rand) interface{} {
	obj, ok := data.(map[string]interface{})
	if !ok {
		return data
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		obj[randomString(rng, 5)] = "fuzz"
		return obj
	}

	op := rng.Intn(4)
	switch op {
	case 0: // add key
		obj[randomString(rng, 5+rng.Intn(10))] = randomString(rng, 10)
	case 1: // remove key
		delete(obj, keys[rng.Intn(len(keys))])
	case 2: // duplicate key (different value)
		key := keys[rng.Intn(len(keys))]
		obj[key+"_dup"] = obj[key]
	case 3: // rename key
		key := keys[rng.Intn(len(keys))]
		val := obj[key]
		delete(obj, key)
		obj[randomString(rng, len(key))] = val
	}
	return obj
}

func (m *JSONMutator) arrayMutation(data interface{}, rng *rand.Rand) interface{} {
	switch v := data.(type) {
	case []interface{}:
		return m.mutateArray(v, rng)
	case map[string]interface{}:
		// Find an array field to mutate
		for k, val := range v {
			if arr, ok := val.([]interface{}); ok {
				v[k] = m.mutateArray(arr, rng)
				return v
			}
		}
		// No array found, wrap a value as array
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			v[key] = []interface{}{v[key]}
		}
		return v
	default:
		return []interface{}{data, data}
	}
}

func (m *JSONMutator) mutateArray(arr []interface{}, rng *rand.Rand) []interface{} {
	op := rng.Intn(4)
	switch op {
	case 0: // duplicate element
		if len(arr) > 0 {
			idx := rng.Intn(len(arr))
			arr = append(arr, arr[idx])
		}
	case 1: // insert mixed type
		arr = append(arr, m.substituteType(nil, rng))
	case 2: // remove element
		if len(arr) > 1 {
			idx := rng.Intn(len(arr))
			arr = append(arr[:idx], arr[idx+1:]...)
		}
	case 3: // make empty
		arr = []interface{}{}
	}
	return arr
}

func (m *JSONMutator) boundaryValues(data interface{}, rng *rand.Rand) interface{} {
	boundaryNumbers := []float64{
		0, -0, 1, -1,
		math.MaxFloat64, -math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		float64(math.MaxInt32), float64(math.MinInt32),
		float64(math.MaxInt64),
		math.NaN(), math.Inf(1), math.Inf(-1),
		1e308, -1e308, 1e-308,
	}

	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			if _, ok := v[key].(float64); ok {
				v[key] = boundaryNumbers[rng.Intn(len(boundaryNumbers))]
			} else {
				v[key] = boundaryNumbers[rng.Intn(len(boundaryNumbers))]
			}
		}
		return v
	case []interface{}:
		if len(v) > 0 {
			v[rng.Intn(len(v))] = boundaryNumbers[rng.Intn(len(boundaryNumbers))]
		}
		return v
	default:
		return boundaryNumbers[rng.Intn(len(boundaryNumbers))]
	}
}

func (m *JSONMutator) depthStress(data interface{}, rng *rand.Rand) interface{} {
	depth := 20 + rng.Intn(80) // 20-100 levels deep
	nested := buildNestedObject(depth, rng)

	switch v := data.(type) {
	case map[string]interface{}:
		v["_fuzz_deep"] = nested
		return v
	default:
		return map[string]interface{}{"_fuzz_deep": nested, "_original": data}
	}
}

func buildNestedObject(depth int, rng *rand.Rand) interface{} {
	if depth <= 0 {
		return randomString(rng, 5)
	}
	if rng.Intn(2) == 0 {
		return map[string]interface{}{"n": buildNestedObject(depth-1, rng)}
	}
	return []interface{}{buildNestedObject(depth-1, rng)}
}

func (m *JSONMutator) stringMutation(data interface{}, rng *rand.Rand) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			if _, ok := v[key].(string); ok {
				v[key] = fuzzStrings[rng.Intn(len(fuzzStrings))]
			}
		}
		return v
	default:
		return fuzzStrings[rng.Intn(len(fuzzStrings))]
	}
}
