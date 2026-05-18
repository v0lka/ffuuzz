import { NavLink, Outlet } from "react-router-dom";
import {
    LayoutDashboard,
    Disc3,
    Rocket,
    AlertTriangle,
    Sun,
    Moon,
    Activity,
    Menu,
} from "lucide-react";
import { useHealth } from "@/hooks/queries";
import { useEffect, useState } from "react";

const navItems = [
    { to: "/", icon: LayoutDashboard, label: "Dashboard" },
    { to: "/recordings", icon: Disc3, label: "Recordings" },
    { to: "/campaigns", icon: Rocket, label: "Campaigns" },
    { to: "/findings", icon: AlertTriangle, label: "Findings" },
];

function ThemeToggle() {
    const [dark, setDark] = useState(true);

    useEffect(() => {
        const current = document.documentElement.getAttribute("data-theme");
        setDark(current !== "light");
    }, []);

    const toggle = () => {
        const next = dark ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", next);
        setDark(!dark);
    };

    return (
        <button className="btn btn-ghost btn-sm btn-square" onClick={toggle}>
            {dark ? <Sun size={18} /> : <Moon size={18} />}
        </button>
    );
}

function HealthDot() {
    const { data } = useHealth();
    const ok = data?.status === "ok";

    return (
        <div className="flex items-center gap-2 text-sm opacity-70">
            <Activity size={14} />
            <span
                className={`badge badge-xs ${ok ? "badge-success" : "badge-error"}`}
            />
            <span>{ok ? "Healthy" : "Degraded"}</span>
        </div>
    );
}

export default function Layout() {
    return (
        <div className="drawer lg:drawer-open">
            <input id="sidebar" type="checkbox" className="drawer-toggle" />
            <div className="drawer-content flex flex-col min-h-screen">
                {/* Top bar */}
                <header className="navbar bg-base-200 lg:hidden">
                    <label
                        htmlFor="sidebar"
                        className="btn btn-ghost drawer-button lg:hidden"
                    >
                        <Menu size={20} />
                    </label>
                    <span className="text-lg font-bold tracking-wider">FFUUZZ</span>
                </header>

                {/* Main content */}
                <main className="flex-1 p-4 md:p-6 lg:p-8">
                    <Outlet />
                </main>
            </div>

            {/* Sidebar */}
            <div className="drawer-side z-40">
                <label htmlFor="sidebar" className="drawer-overlay" />
                <aside className="bg-base-200 w-64 min-h-full flex flex-col">
                    <div className="p-4 flex items-center justify-between">
                        <span className="text-xl font-bold tracking-widest">FFUUZZ</span>
                        <ThemeToggle />
                    </div>
                    <ul className="menu flex-1 px-2 gap-1">
                        {navItems.map((item) => (
                            <li key={item.to}>
                                <NavLink
                                    to={item.to}
                                    end={item.to === "/"}
                                    className={({ isActive }) =>
                                        isActive ? "menu-active" : ""
                                    }
                                >
                                    <item.icon size={18} />
                                    {item.label}
                                </NavLink>
                            </li>
                        ))}
                    </ul>
                    <div className="p-4 border-t border-base-300">
                        <HealthDot />
                    </div>
                </aside>
            </div>
        </div>
    );
}
