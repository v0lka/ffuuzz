import { Link } from "react-router-dom";

export default function NotFoundPage() {
    return (
        <div className="flex flex-col items-center justify-center min-h-[50vh] text-center">
            <h1 className="text-6xl font-bold opacity-20">404</h1>
            <p className="text-lg mt-4">Page not found</p>
            <Link to="/" className="btn btn-primary btn-sm mt-6">
                Go to Dashboard
            </Link>
        </div>
    );
}
