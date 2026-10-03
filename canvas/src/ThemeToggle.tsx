import { useEffect, useState } from "react";

export function ThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.dataset.theme === "dark");

  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    try {
      localStorage.setItem("preflight-theme", dark ? "dark" : "light");
    } catch {
      // The switch still works when browser storage is unavailable.
    }
  }, [dark]);

  return (
    <button className="theme-toggle" onClick={() => setDark((value) => !value)}
      aria-label={dark ? "Switch to light mode" : "Switch to dark mode"}
      aria-pressed={dark} title={dark ? "Switch to light mode" : "Switch to dark mode"}>
      <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        {dark ? <><circle cx="12" cy="12" r="4" /><path d="M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1.5 1.5 M17.5 17.5L19 19 M5 19l1.5-1.5 M17.5 6.5L19 5" /></>
          : <path d="M20.5 13A9 9 0 0111 3.5 9 9 0 1020.5 13z" />}
      </svg>
    </button>
  );
}
