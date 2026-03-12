import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "react-router-dom";
import { ErrorBoundary, type FallbackProps } from "react-error-boundary";
import { router } from "./router";

const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            retry: 1,
            staleTime: 30_000,
            refetchOnWindowFocus: false,
        },
    },
});

function ErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
    return (
        <div className="flex flex-col items-center justify-center min-h-screen p-8 text-center">
            <h1 className="text-2xl font-bold mb-4">Something went wrong</h1>
            <pre className="bg-base-300 rounded-box p-4 text-xs mb-4 max-w-lg overflow-auto">
                {error instanceof Error ? error.message : String(error)}
            </pre>
            <button className="btn btn-primary" onClick={resetErrorBoundary}>
                Try again
            </button>
        </div>
    );
}

export default function App() {
    return (
        <ErrorBoundary FallbackComponent={ErrorFallback}>
            <QueryClientProvider client={queryClient}>
                <RouterProvider router={router} />
            </QueryClientProvider>
        </ErrorBoundary>
    );
}
