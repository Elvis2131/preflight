// AwsIcon draws one official AWS icon at a fixed SQUARE size, so the icon's proportions are
// never changed (AWS's own rule: do not alter the icons). alt is empty because the icon is
// decorative — the node's label already names the service — and it is not draggable, so
// dragging a node never drags the image.
export function AwsIcon({ src, size = 22, title, kind }: { src: string; size?: number; title?: string; kind?: "service" | "category" | "group" }) {
  return (
    <img
      src={src}
      width={size}
      height={size}
      alt=""
      title={title}
      draggable={false}
      loading="lazy"
      decoding="async"
      style={{ flex: "none", display: "block" }}
      data-testid="aws-icon"
      data-icon-kind={kind}
    />
  );
}
