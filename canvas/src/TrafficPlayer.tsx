import type { JourneyFlowResult } from "./api";
import type { DeclaredJourney } from "./workloadTypes";
import type { TrafficPlayback } from "./useTrafficPlayback";
import { trafficStepStopped } from "./trafficPlayback";

interface Props {
  journeys: DeclaredJourney[];
  selectedJourneyID: string | null;
  onSelectJourney: (id: string) => void;
  flow: JourneyFlowResult | null;
  playback: TrafficPlayback;
  labelForNode: (id: string) => string;
  onClose: () => void;
  onFocus: () => void;
  busy: boolean;
}

export function TrafficPlayer({ journeys, selectedJourneyID, onSelectJourney, flow, playback: p, labelForNode, onClose, onFocus, busy }: Props) {
  const stopped = trafficStepStopped(p.active);
  const dns = p.active?.dns;
  return (
    <section className="traffic-player" aria-label="Traffic playback" data-testid="traffic-player">
      <div className="traffic-player-controls">
        <label>Traffic journey
          <select aria-label="Traffic journey" value={selectedJourneyID ?? ""} onChange={(e) => onSelectJourney(e.target.value)}>
            <option value="" disabled>Choose a journey</option>
            {journeys.map((j) => <option key={j.id} value={j.id}>{j.name}</option>)}
          </select>
        </label>
        <button className="primary-action" data-testid="traffic-play" disabled={!flow || !p.steps.length || busy || p.reducedMotion || !!stopped} onClick={() => p.playing ? p.pause() : p.start()}>
          {p.playing ? "Pause" : p.complete ? "Play again" : "Play"}
        </button>
        <button aria-label="Previous hop" disabled={p.index === 0 || busy} onClick={() => p.goTo(p.index - 1)}>←</button>
        <button aria-label="Next hop" disabled={!flow || !p.steps.length || p.complete || busy} onClick={() => p.goTo(p.index + 1)}>Next hop →</button>
        <button disabled={!flow || !p.steps.length || busy} onClick={() => p.start(flow, true)}>Replay</button>
        <button disabled={!flow || !p.steps.length || busy} onClick={onFocus}>Focus path</button>
        <label className="traffic-speed">Speed
          <select aria-label="Playback speed" value={p.speed} onChange={(e) => p.changeSpeed(Number(e.target.value))} disabled={p.reducedMotion}>
            <option value={0.5}>0.5×</option><option value={1}>1×</option><option value={2}>2×</option>
          </select>
        </label>
        <span className="traffic-playback-note">{p.reducedMotion ? "Reduced motion · use Next hop" : "Visual replay · not network timing"}</span>
        <button className="traffic-close" aria-label="Close traffic playback" onClick={onClose}>×</button>
      </div>
      <div className={`traffic-player-progress${stopped ? " traffic-stopped" : ""}`} data-testid="traffic-progress" aria-live="polite">
        <strong>{busy ? "Checking traffic…" : !flow ? "Run a baseline to check this journey." : !p.steps.length ? flow.BlockedReason || "No traffic hops returned." : p.complete ? "Replay complete" : `${dns ? "DNS lookup" : stopped ? "Stopped at hop" : "Hop"} ${p.index + 1} of ${p.steps.length}`}</strong>
        {p.active && <span className="traffic-current-hop">{dns ? `${labelForNode(dns.From)} → ${labelForNode(dns.To)}` : p.active.hops.map((h) => `${labelForNode(h.From)} → ${labelForNode(h.To)}`).join(" · ")}</span>}
        <div className="traffic-step-buttons" aria-label="Playback hops">
          {p.steps.map((s, i) => <button key={i} aria-label={`Show hop ${i + 1}`} title={s.dns ? "DNS lookup · declared architecture" : "Traffic check · simulation result"} aria-current={p.index === i ? "step" : undefined} className={trafficStepStopped(s) ? "stopped" : ""} onClick={() => p.goTo(i)} disabled={busy}>{i + 1}</button>)}
        </div>
      </div>
      {dns && <p className="traffic-dns-note">DNS lookup via the client's resolver returns the load balancer's address. This step illustrates the declared architecture; DNS resolution is not assessed. The HTTPS request follows separately.</p>}
      {p.active?.hops.filter((h) => !h.Allowed).map((h, i) => <p className="traffic-hop-reason" key={i}>{h.Reason}</p>)}
    </section>
  );
}
