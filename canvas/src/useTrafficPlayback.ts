import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { JourneyFlowResult } from "./api";
import { trafficSteps, trafficStepCanPlay, type DnsLookup } from "./trafficPlayback";

const HOP_DURATION_MS = 1800; // Visual pacing, never predicted network latency.
const subscribeMotion = (onChange: () => void) => {
  const media = window.matchMedia("(prefers-reduced-motion: reduce)");
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
};
const getReducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

export function useTrafficPlayback(flow: JourneyFlowResult | null, enabled: boolean, dns: DnsLookup | null = null) {
  const reducedMotion = useSyncExternalStore(subscribeMotion, getReducedMotion, () => false);
  const steps = useMemo(() => trafficSteps(flow, dns), [flow, dns]);
  const [speed, setSpeed] = useState(1);
  const [state, setState] = useState({ flow, dns, index: 0, playing: false, run: 0 });
  // New assessments/selections start paused. Old timers/frames cannot paint a new result.
  const index = state.flow === flow && state.dns === dns ? state.index : 0;
  const playing = enabled && state.flow === flow && state.dns === dns && state.playing && !reducedMotion;
  const active = enabled ? steps[index] ?? null : null;
  const elapsed = useRef({ flow, index: -1, run: -1, ms: 0 });

  useEffect(() => {
    if (!playing || !flow || !active) return;
    if (elapsed.current.flow !== flow || elapsed.current.index !== index || elapsed.current.run !== state.run) {
      elapsed.current = { flow, index, run: state.run, ms: 0 };
    }
    const started = performance.now();
    const timer = setTimeout(() => {
      setState((s) => {
        if (s.flow !== flow || s.dns !== dns || s.index !== index || s.run !== state.run) return s;
        const next = index + 1;
        return { ...s, index: next, playing: trafficStepCanPlay(steps[next]) };
      });
    }, Math.max(0, HOP_DURATION_MS - elapsed.current.ms) / speed);
    return () => {
      clearTimeout(timer);
      elapsed.current.ms += (performance.now() - started) * speed;
    };
  }, [playing, flow, dns, active, index, state.run, steps, speed]);

  const start = (result: JourneyFlowResult | null = flow, restart = false) => {
    if (!result) return;
    const resultSteps = trafficSteps(result, dns);
    setState((s) => {
      const reset = restart || s.flow !== result || s.dns !== dns || s.index >= resultSteps.length;
      const nextIndex = reset ? 0 : s.index;
      return { flow: result, dns, index: nextIndex, playing: !reducedMotion && trafficStepCanPlay(resultSteps[nextIndex]), run: reset ? s.run + 1 : s.run };
    });
  };
  const goTo = (next: number) => setState((s) => ({ flow, dns, index: Math.max(0, Math.min(next, steps.length)), playing: false, run: s.run + 1 }));
  return {
    steps, active, index, playing, reducedMotion, speed,
    complete: steps.length > 0 && index >= steps.length,
    duration: HOP_DURATION_MS / speed,
    packetKey: `${index}-${state.run}`,
    start,
    pause: () => setState((s) => ({ ...s, playing: false })),
    goTo,
    changeSpeed: (value: number) => {
      setSpeed(value);
      // Restart the current visual hop at the new pace; this changes no result.
      setState((s) => ({ ...s, run: s.run + 1 }));
    },
  };
}

export type TrafficPlayback = ReturnType<typeof useTrafficPlayback>;
