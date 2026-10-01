import { AwsIcon } from "./AwsIcon";
import { iconDetailsForService, labelForService } from "./awsIcons";

function initials(label: string): string {
  const words = label.replace(/^Amazon\s+/i, "").replace(/^AWS\s+/i, "").split(/\s+/).filter(Boolean);
  if (words.length === 1) return words[0].length <= 4 ? words[0] : words[0].slice(0, 2);
  return (words[0]?.[0] ?? "?") + (words[1]?.[0] ?? "");
}

export function ServiceMark({ serviceID, size = 22 }: { serviceID?: string; size?: number }) {
  const icon = iconDetailsForService(serviceID);
  if (icon) {
    return <AwsIcon src={icon.src} size={size} title={icon.title} kind={icon.kind} />;
  }
  const label = serviceID ? labelForService(serviceID) : "Service";
  return (
    <span
      aria-hidden
      title={label}
      data-testid="service-mark"
      style={{
        width: size,
        height: size,
        flex: "none",
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        border: "1px solid #cbd5e1",
        borderRadius: 5,
        background: "#f8fafc",
        color: "#475569",
        fontSize: Math.max(9, Math.round(size * 0.42)),
        fontWeight: 700,
        lineHeight: 1,
      }}
    >
      {initials(label).toUpperCase()}
    </span>
  );
}
