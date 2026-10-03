import { useEffect, useRef } from "react";
import { EDGE_TYPES, EDGE_TYPE_GUIDES, EDGE_TYPE_LABELS, type EdgeType } from "./goldenVocabulary";

interface Props {
  source: string;
  target: string;
  currentType?: EdgeType;
  anchor: { x: number; y: number };
  onChoose: (type: EdgeType) => void;
  onClose: () => void;
}

export function ConnectionPicker({ source, target, currentType, anchor, onChoose, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const element = dialog.current!;
    element.showModal();
    const position = () => {
      const box = element.getBoundingClientRect();
      element.style.left = `${Math.max(12, Math.min(anchor.x + 16, window.innerWidth - box.width - 12))}px`;
      element.style.top = `${Math.max(12, Math.min(anchor.y - 30, window.innerHeight - box.height - 12))}px`;
    };
    position();
    window.addEventListener("resize", position);
    element.querySelector<HTMLButtonElement>(currentType ? `[data-connection-type="${currentType}"]` : "[data-connection-type]")?.focus();
    return () => { window.removeEventListener("resize", position); element.close(); };
  }, [currentType, anchor.x, anchor.y]);

  const width = Math.min(340, window.innerWidth - 24);
  const left = Math.max(12, Math.min(anchor.x + 16, window.innerWidth - width - 12));
  const top = Math.max(12, Math.min(anchor.y - 30, window.innerHeight - 540));
  return <dialog ref={dialog} className="connection-picker" aria-labelledby="connection-picker-title" aria-describedby="connection-picker-endpoints"
    style={{ left, top, width }} onCancel={onClose}
    onClick={(event) => {
      if (event.target !== event.currentTarget) return;
      const box = event.currentTarget.getBoundingClientRect();
      if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) onClose();
    }}>
    <div className="connection-picker-heading"><div><span className="eyebrow">{currentType ? "EDIT CONNECTION" : "NEW CONNECTION"}</span>
      <h2 id="connection-picker-title">Choose the connection type</h2></div>
      <button className="connection-picker-close" aria-label="Cancel connection" onClick={onClose}>×</button>
    </div>
    <div className="connection-endpoints" id="connection-picker-endpoints"><span title={source}>{source}</span><span aria-hidden>→</span><span title={target}>{target}</span></div>
    <p className="connection-picker-help">How does the source relate to the destination?</p>
    <div className="connection-options">
      {EDGE_TYPES.map((type, index) => <button key={type} data-connection-type={type} aria-pressed={type === currentType}
        className="connection-option" onClick={() => onChoose(type)}>
        <span className="connection-option-icon" aria-hidden>{["↗", "→", "⇄", "⌘", "⇢", "⊏"][index]}</span>
        <span><strong>{EDGE_TYPE_LABELS[type]}</strong><small>{EDGE_TYPE_GUIDES[type]}</small></span>
        {type === currentType && <span className="connection-current" aria-hidden>✓</span>}
      </button>)}
    </div>
    <div className="connection-picker-footer">{currentType ? "Changes apply to this connection only." : "Choose a type to create the connection. Esc to cancel."}</div>
  </dialog>;
}
