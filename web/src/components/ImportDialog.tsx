import { useState, useCallback, type ChangeEvent } from "react";
import { Upload } from "lucide-react";
import { useImportRecordings } from "@/hooks/queries";
import type { RecordingSession } from "@/types/api";

interface Props {
    open: boolean;
    onClose: () => void;
}

export function ImportDialog({ open, onClose }: Props) {
    const [sessions, setSessions] = useState<RecordingSession[]>([]);
    const [parseError, setParseError] = useState<string | null>(null);
    const importMutation = useImportRecordings();

    const handleFile = useCallback((e: ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        const reader = new FileReader();
        reader.onload = () => {
            try {
                const data = JSON.parse(reader.result as string);
                const arr = Array.isArray(data)
                    ? data
                    : data.sessions
                        ? data.sessions
                        : [data];
                const valid = (arr as unknown[]).every(
                    (s: unknown) =>
                        s != null && typeof s === "object" && "id" in s && "target" in s,
                );
                if (!valid) {
                    setParseError("Invalid recording session format");
                    setSessions([]);
                    return;
                }
                setSessions(arr as RecordingSession[]);
                setParseError(null);
            } catch {
                setParseError("Failed to parse JSON file");
                setSessions([]);
            }
        };
        reader.readAsText(file);
    }, []);

    const handleImport = () => {
        importMutation.mutate(sessions, {
            onSuccess: () => {
                setSessions([]);
                onClose();
            },
        });
    };

    const handleClose = () => {
        setSessions([]);
        setParseError(null);
        importMutation.reset();
        onClose();
    };

    if (!open) return null;

    return (
        <dialog className="modal modal-open">
            <div className="modal-box">
                <h3 className="font-bold text-lg flex items-center gap-2">
                    <Upload size={20} />
                    Import Recordings
                </h3>

                <div className="py-4">
                    <input
                        type="file"
                        accept=".json"
                        className="file-input file-input-bordered w-full"
                        onChange={handleFile}
                    />

                    {parseError && (
                        <p className="text-error text-sm mt-2">{parseError}</p>
                    )}

                    {sessions.length > 0 && (
                        <p className="text-sm mt-2">
                            {sessions.length} session(s) ready to import
                        </p>
                    )}

                    {importMutation.isSuccess && importMutation.data && (
                        <div className="mt-2 text-sm">
                            <p className="text-success">
                                Imported: {importMutation.data.imported}
                            </p>
                            {importMutation.data.skipped > 0 && (
                                <p className="text-warning">
                                    Skipped: {importMutation.data.skipped}
                                </p>
                            )}
                            {importMutation.data.failed > 0 && (
                                <p className="text-error">
                                    Failed: {importMutation.data.failed}
                                </p>
                            )}
                        </div>
                    )}

                    {importMutation.isError && (
                        <p className="text-error text-sm mt-2">
                            {importMutation.error.message}
                        </p>
                    )}
                </div>

                <div className="modal-action">
                    <button className="btn" onClick={handleClose}>
                        Cancel
                    </button>
                    <button
                        className="btn btn-primary"
                        disabled={sessions.length === 0 || importMutation.isPending}
                        onClick={handleImport}
                    >
                        {importMutation.isPending ? (
                            <span className="loading loading-spinner loading-sm" />
                        ) : (
                            "Import"
                        )}
                    </button>
                </div>
            </div>
            <div className="modal-backdrop" onClick={handleClose} />
        </dialog>
    );
}
