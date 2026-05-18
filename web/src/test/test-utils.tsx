import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { render, type RenderOptions } from "@testing-library/react";
import type { ReactElement } from "react";

export function createTestQueryClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: {
                retry: false,
                gcTime: 0,
                staleTime: 0,
            },
            mutations: {
                retry: false,
            },
        },
    });
}

interface AllTheProvidersProps {
    children: React.ReactNode;
    initialEntries?: string[];
}

function AllTheProviders({ children, initialEntries }: AllTheProvidersProps) {
    const queryClient = createTestQueryClient();
    return (
        <QueryClientProvider client={queryClient}>
            <MemoryRouter
                initialEntries={initialEntries ?? ["/"]}
                basename="/ui"
            >
                {children}
            </MemoryRouter>
        </QueryClientProvider>
    );
}

interface CustomRenderOptions extends Omit<RenderOptions, "wrapper"> {
    initialEntries?: string[];
}

export function renderWithProviders(
    ui: ReactElement,
    options?: CustomRenderOptions,
) {
    return render(ui, {
        wrapper: ({ children }) => (
            <AllTheProviders initialEntries={options?.initialEntries}>
                {children}
            </AllTheProviders>
        ),
        ...options,
    });
}

export * from "@testing-library/react";
export { renderWithProviders as render };
