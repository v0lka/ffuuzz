interface Props {
    offset: number;
    limit: number;
    hasMore: boolean;
    onPageChange: (newOffset: number) => void;
}

export function Pagination({ offset, limit, hasMore, onPageChange }: Props) {
    const page = Math.floor(offset / limit) + 1;

    return (
        <div className="join mt-4">
            <button
                className="join-item btn btn-sm"
                disabled={offset === 0}
                onClick={() => onPageChange(Math.max(0, offset - limit))}
            >
                Previous
            </button>
            <button className="join-item btn btn-sm btn-disabled">
                Page {page}
            </button>
            <button
                className="join-item btn btn-sm"
                disabled={!hasMore}
                onClick={() => onPageChange(offset + limit)}
            >
                Next
            </button>
        </div>
    );
}
