import { AlertCircle } from "lucide-react";

interface Props {
    message: string;
    requestId?: string;
}

export function ErrorAlert({ message, requestId }: Props) {
    return (
        <div role="alert" className="alert alert-error">
            <AlertCircle size={20} />
            <div>
                <p>{message}</p>
                {requestId && (
                    <p className="text-xs opacity-70 mt-1">Request ID: {requestId}</p>
                )}
            </div>
        </div>
    );
}
