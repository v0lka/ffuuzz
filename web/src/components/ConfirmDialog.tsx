interface Props {
    title: string;
    message: string;
    confirmLabel?: string;
    onConfirm: () => void;
    onCancel: () => void;
}

export function ConfirmDialog({
    title,
    message,
    confirmLabel = "Confirm",
    onConfirm,
    onCancel,
}: Props) {
    return (
        <dialog className="modal modal-open">
            <div className="modal-box">
                <h3 className="font-bold text-lg">{title}</h3>
                <p className="py-4">{message}</p>
                <div className="modal-action">
                    <button className="btn" onClick={onCancel}>
                        Cancel
                    </button>
                    <button className="btn btn-error" onClick={onConfirm}>
                        {confirmLabel}
                    </button>
                </div>
            </div>
            <div className="modal-backdrop" onClick={onCancel} />
        </dialog>
    );
}
