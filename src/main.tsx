import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/global.css";

function App() {
  return (
    <main className="app-shell">
      <section className="hero">
        <p className="eyebrow">LLM Relay Entry</p>
        <h1>Hello World</h1>
        <p className="summary">
          Codex relay entry frontend placeholder. This screen keeps the web
          build pipeline ready for future pages.
        </p>
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
