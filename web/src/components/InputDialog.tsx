import { useState } from "react";

interface Props {
    title: string;
    label: string;
    placeholder?: string;
    confirmLabel?: string;
    onConfirm: (value: string) => void;
    onCancel: () => void;
}

export function InputDialog({
    title,
    label,
    placeholder,
    confirmLabel = "Create",
    onConfirm,
    onCancel,
}: Props) {
    const [value, setValue] = useState("");

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (value.trim()) {
            onConfirm(value.trim());
        }
    };

    return (
        <dialog className="modal modal-open">
            <div className="modal-box">
                <h3 className="font-bold text-lg">{title}</h3>
                <form onSubmit={handleSubmit}>
                    <label className="label">
                        <span className="label-text">{label}</span>
                    </label>
                    <input
                        type="text"
                        className="input input-bordered w-full"
                        placeholder={placeholder}
                        value={value}
                        onChange={(e) => setValue(e.target.value)}
                        autoFocus
                    />
                    <div className="modal-action">
                        <button type="button" className="btn" onClick={onCancel}>
                            Cancel
                        </button>
                        <button
                            type="submit"
                            className="btn btn-primary"
                            disabled={!value.trim()}
                        >
                            {confirmLabel}
                        </button>
                    </div>
                </form>
            </div>
            <div className="modal-backdrop" onClick={onCancel} />
        </dialog>
    );
}
