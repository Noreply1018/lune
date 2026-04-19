import { useEffect } from "react";

const PHASE_REVEAL_MS = 3200;

type PhaseRevealProps = {
  text: string;
  onDismiss: () => void;
};

export default function PhaseReveal({ text, onDismiss }: PhaseRevealProps) {
  useEffect(() => {
    const id = window.setTimeout(onDismiss, PHASE_REVEAL_MS);
    return () => window.clearTimeout(id);
  }, [onDismiss]);

  return (
    <div className="pointer-events-none absolute left-1/2 bottom-20 z-10 -translate-x-1/2 text-center">
      <style>{`
        @keyframes lune-phase-reveal {
          0%   { opacity: 0; transform: translateY(-6px); }
          18%  { opacity: 1; transform: translateY(0); }
          78%  { opacity: 1; transform: translateY(0); }
          100% { opacity: 0; transform: translateY(8px); }
        }
      `}</style>
      <span
        className="inline-block text-[15px] tracking-[0.32em]"
        style={{
          fontFamily:
            "'Iowan Old Style','Palatino Linotype','Noto Serif SC','Source Han Serif SC',Georgia,serif",
          color: "rgba(33,40,63,0.78)",
          animation: `lune-phase-reveal ${PHASE_REVEAL_MS}ms ease-out forwards`,
        }}
      >
        {text}
      </span>
    </div>
  );
}
