import { useEffect, useRef } from "react";

export function TemplateConfirmation({ name, onCancel, onProceed }: {
  name: string; onCancel: () => void; onProceed: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const element = dialog.current!;
    element.showModal();
    cancel.current?.focus();
    return () => element.close();
  }, []);

  return <dialog ref={dialog} className="template-confirmation" aria-labelledby="template-confirmation-title"
    aria-describedby="template-confirmation-description" onCancel={onCancel}>
    <span className="eyebrow">CHANGE ARCHITECTURE</span>
    <h2 id="template-confirmation-title">Switch architecture sample?</h2>
    <p id="template-confirmation-description">Opening <strong>{name}</strong> will replace your current services, connections and workload settings.</p>
    <div className="template-confirmation-actions">
      <button ref={cancel} onClick={onCancel}>Keep current design</button>
      <button className="primary-action" onClick={onProceed}>Switch sample</button>
    </div>
  </dialog>;
}
