import type { ReactNode } from "react";

interface Props {
    icon?: ReactNode;
    title: string;
    description?: string;
    action?: ReactNode;
}

export function EmptyState({ icon, title, description, action }: Props) {
    return (
        <div className="flex flex-col items-center justify-center p-12 text-center opacity-60">
            {icon && <div className="mb-4 text-4xl">{icon}</div>}
            <h3 className="text-lg font-semibold">{title}</h3>
            {description && <p className="text-sm mt-1">{description}</p>}
            {action && <div className="mt-4">{action}</div>}
        </div>
    );
}
