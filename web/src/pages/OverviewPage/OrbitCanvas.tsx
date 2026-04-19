import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getMoonPhase } from "./moonPhase";

export type OrbitAccount = {
  id: number;
  label: string;
  health: "healthy" | "degraded" | "error" | "disabled" | "unknown";
};

export type OrbitPool = {
  id: number;
  label: string;
  enabled: boolean;
  health: "healthy" | "degraded" | "error" | "disabled" | "unknown";
  accounts: OrbitAccount[];
};

export type OrbitCanvasProps = {
  pools: OrbitPool[];
  moonTone: "calm" | "warning" | "critical";
  zoomLevel: "far" | "mid" | "near";
  moonFace?: React.ReactNode;
  onAccountClick?: (poolId: number, accountId: number) => void;
  onMoonClick?: () => void;
  moonCursor?: "pointer" | "default";
};

const HEALTH_COLOR: Record<OrbitAccount["health"], string> = {
  healthy: "#5d9f87",
  degraded: "#c09a55",
  error: "#be7476",
  disabled: "#98a0b7",
  unknown: "#98a0b7",
};

const HEALTH_LABEL: Record<OrbitAccount["health"], string> = {
  healthy: "健康",
  degraded: "降级",
  error: "异常",
  disabled: "已停用",
  unknown: "未知",
};

const MOON_TONE_GRADIENT: Record<OrbitCanvasProps["moonTone"], { halo: string; rim: string }> = {
  calm: { halo: "rgba(134,125,193,0.30)", rim: "rgba(134,125,193,0.14)" },
  warning: { halo: "rgba(192,154,85,0.32)", rim: "rgba(192,154,85,0.16)" },
  critical: { halo: "rgba(190,116,118,0.38)", rim: "rgba(190,116,118,0.18)" },
};

const ZOOM_SCALE: Record<OrbitCanvasProps["zoomLevel"], number> = {
  far: 0.58,
  mid: 1,
  near: 1.7,
};

const MOON_BASE_RADIUS = 64;

function seededOffset(seed: number, max: number): number {
  let x = Math.sin(seed * 9301 + 49297) * 233280;
  x = x - Math.floor(x);
  return x * max;
}

function arcPath(rx: number, ry: number, half: "back" | "front") {
  const sweep = half === "back" ? 0 : 1;
  return `M ${rx} 0 A ${rx} ${ry} 0 0 ${sweep} ${-rx} 0`;
}

type DotPosition = {
  poolIdx: number;
  accountIdx: number;
  pool: OrbitPool;
  account: OrbitAccount;
  x: number;
  y: number;
  depth: number;
  breathPhase: number;
  breathPeriod: number;
};

type HoverState = { poolIdx: number; accountIdx: number } | null;

export default function OrbitCanvas({
  pools,
  moonTone,
  zoomLevel,
  moonFace,
  onAccountClick,
  onMoonClick,
  moonCursor = "default",
}: OrbitCanvasProps) {
  const [frameTime, setFrameTime] = useState(0);
  const [hover, setHover] = useState<HoverState>(null);
  const [tilt, setTilt] = useState(0.42);
  const [moonOffset, setMoonOffset] = useState({ x: 0, y: 0 });
  const containerRef = useRef<HTMLDivElement>(null);
  const elapsedRef = useRef(0);
  const lastTickRef = useRef<number | null>(null);
  const pausedRef = useRef(false);
  const dragRef = useRef<{ startX: number; startY: number; startTilt: number; startOffset: { x: number; y: number } } | null>(null);
  const springbackRef = useRef<number>(0);
  const [isDragging, setIsDragging] = useState(false);

  useEffect(() => {
    pausedRef.current = hover !== null || isDragging;
  }, [hover, isDragging]);

  useEffect(() => {
    let raf = 0;
    const loop = (now: number) => {
      if (lastTickRef.current == null) lastTickRef.current = now;
      const dt = now - lastTickRef.current;
      lastTickRef.current = now;
      if (!pausedRef.current) {
        elapsedRef.current += dt;
        setFrameTime(elapsedRef.current);
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, []);

  const moonPhase = useMemo(() => getMoonPhase(), []);
  const moonRadius = MOON_BASE_RADIUS * ZOOM_SCALE[zoomLevel];
  const tone = MOON_TONE_GRADIENT[moonTone];

  const orbitPools = useMemo(() => {
    const enabled = pools.filter((p) => p.enabled);
    const baseRadius = 160 * ZOOM_SCALE[zoomLevel];
    const step = 72 * ZOOM_SCALE[zoomLevel];
    return enabled.map((pool, i) => {
      const radius = baseRadius + i * step;
      const duration = 120 + i * 48;
      const direction: 1 | -1 = i % 2 === 0 ? 1 : -1;
      const phase = seededOffset(pool.id, Math.PI * 2);
      const accountsWithBreath = pool.accounts.map((acc) => {
        const breathSeed = pool.id * 31 + acc.id;
        return {
          acc,
          breathPeriod: 2400 + seededOffset(breathSeed, 2600),
          breathPhase: seededOffset(breathSeed + 1, Math.PI * 2),
        };
      });
      return { pool, radius, duration, direction, phase, accountsWithBreath };
    });
  }, [pools, zoomLevel]);

  const dots: DotPosition[] = useMemo(() => {
    const result: DotPosition[] = [];
    orbitPools.forEach(({ pool, radius, duration, direction, phase, accountsWithBreath }, poolIdx) => {
      const rotation = (frameTime / 1000 / duration) * 2 * Math.PI * direction;
      const n = Math.max(accountsWithBreath.length, 1);
      accountsWithBreath.forEach(({ acc, breathPeriod, breathPhase }, i) => {
        const angle = (i / n) * 2 * Math.PI + phase + rotation;
        const x = radius * Math.cos(angle);
        const y = radius * Math.sin(angle) * tilt + moonOffset.y * 0.15;
        const depth = Math.sin(angle);
        result.push({
          poolIdx,
          accountIdx: i,
          pool,
          account: acc,
          x: x + moonOffset.x * 0.15,
          y,
          depth,
          breathPhase,
          breathPeriod,
        });
      });
    });
    return result;
  }, [orbitPools, frameTime, tilt, moonOffset]);

  const backDots = dots.filter((d) => d.depth < 0);
  const frontDots = dots.filter((d) => d.depth >= 0);
  const activePoolIdx = hover?.poolIdx ?? null;

  const hoveredDot = hover
    ? dots.find((d) => d.poolIdx === hover.poolIdx && d.accountIdx === hover.accountIdx)
    : null;

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if ((e.target as Element).closest("[data-orbit-hit]")) return;
    if (springbackRef.current) {
      cancelAnimationFrame(springbackRef.current);
      springbackRef.current = 0;
    }
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      startTilt: tilt,
      startOffset: { ...moonOffset },
    };
    setIsDragging(true);
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
  }, [tilt, moonOffset]);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    const d = dragRef.current;
    if (!d) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    const nextTilt = Math.max(0.12, Math.min(0.72, d.startTilt + dy * 0.0018));
    setTilt(nextTilt);
    setMoonOffset({
      x: Math.max(-120, Math.min(120, d.startOffset.x + dx * 0.6)),
      y: Math.max(-80, Math.min(80, d.startOffset.y + dy * 0.4)),
    });
  }, []);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    if (!dragRef.current) return;
    dragRef.current = null;
    setIsDragging(false);
    try {
      (e.currentTarget as Element).releasePointerCapture(e.pointerId);
    } catch {}
    // spring back with damped interpolation — store each frame in springbackRef
    // so handlePointerDown can cancel a still-running springback.
    const start = performance.now();
    const duration = 420;
    const tiltStart = tilt;
    const offsetStart = { ...moonOffset };
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - t, 3);
      setTilt(tiltStart + (0.42 - tiltStart) * eased);
      setMoonOffset({
        x: offsetStart.x * (1 - eased),
        y: offsetStart.y * (1 - eased),
      });
      if (t < 1) {
        springbackRef.current = requestAnimationFrame(step);
      } else {
        springbackRef.current = 0;
      }
    };
    springbackRef.current = requestAnimationFrame(step);
  }, [tilt, moonOffset]);

  const visibleRingOpacity = zoomLevel === "far" ? 0.35 : zoomLevel === "near" ? 0.2 : 1;

  return (
    <div
      ref={containerRef}
      className="relative h-full w-full"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      style={{ cursor: isDragging ? "grabbing" : "grab", touchAction: "none" }}
    >
      <svg
        viewBox="-500 -400 1000 800"
        preserveAspectRatio="xMidYMid slice"
        className="absolute inset-0 h-full w-full"
        role="img"
        aria-label="Lune orbit"
      >
        <defs>
          <radialGradient id="lune-sphere" cx="32%" cy="26%" r="78%">
            <stop offset="0" stopColor="#fffdf2" />
            <stop offset="0.18" stopColor="#f6f0e0" />
            <stop offset="0.5" stopColor="#cfc4e0" />
            <stop offset="0.82" stopColor="#7a72a6" />
            <stop offset="1" stopColor="#4a4370" />
          </radialGradient>
          <radialGradient id="lune-specular" cx="50%" cy="50%" r="50%">
            <stop offset="0" stopColor="rgba(255,253,240,0.75)" />
            <stop offset="0.6" stopColor="rgba(255,253,240,0.08)" />
            <stop offset="1" stopColor="rgba(255,253,240,0)" />
          </radialGradient>
          <radialGradient id="lune-shadow" cx="50%" cy="50%" r="50%">
            <stop offset="0" stopColor="rgba(50,45,80,0.38)" />
            <stop offset="0.6" stopColor="rgba(50,45,80,0.1)" />
            <stop offset="1" stopColor="rgba(50,45,80,0)" />
          </radialGradient>
          <radialGradient id="lune-halo" cx="50%" cy="50%" r="50%">
            <stop offset="0" stopColor={tone.halo} />
            <stop offset="0.55" stopColor={tone.rim} />
            <stop offset="1" stopColor="rgba(134,125,193,0)" />
          </radialGradient>
          <radialGradient id="lune-warm" cx="50%" cy="50%" r="50%">
            <stop offset="0" stopColor="rgba(255,240,205,0.22)" />
            <stop offset="0.6" stopColor="rgba(255,240,205,0.04)" />
            <stop offset="1" stopColor="rgba(255,240,205,0)" />
          </radialGradient>
          <filter id="orbit-dot-glow" x="-150%" y="-150%" width="400%" height="400%">
            <feGaussianBlur stdDeviation={3.2} />
          </filter>
          <clipPath id="moon-clip">
            <circle cx={0} cy={0} r={moonRadius} />
          </clipPath>
        </defs>

        <g transform={`translate(${moonOffset.x}, ${moonOffset.y})`}>
          {/* warm outer glow */}
          <circle cx={0} cy={0} r={moonRadius * 4.2} fill="url(#lune-warm)" />
          <circle cx={0} cy={0} r={moonRadius * 2.1} fill="url(#lune-halo)" />

          {/* back arcs */}
          {orbitPools.map(({ pool, radius }, i) => (
            <path
              key={`back-ring-${pool.id}`}
              d={arcPath(radius, radius * tilt, "back")}
              fill="none"
              stroke="rgba(134,125,193,0.22)"
              strokeWidth={activePoolIdx === i ? 1.2 : 0.85}
              opacity={(activePoolIdx === i ? 0.78 : 0.55) * visibleRingOpacity}
            />
          ))}

          {/* back dots */}
          {backDots.map((dot) => (
            <AccountDot
              key={`back-${dot.poolIdx}-${dot.accountIdx}`}
              dot={dot}
              frameTime={frameTime}
              highlighted={
                hover?.poolIdx === dot.poolIdx && hover?.accountIdx === dot.accountIdx
              }
              onEnter={() => setHover({ poolIdx: dot.poolIdx, accountIdx: dot.accountIdx })}
              onLeave={() => setHover(null)}
              onClick={() => onAccountClick?.(dot.pool.id, dot.account.id)}
            />
          ))}

          {/* shadow under moon on the plane */}
          <ellipse
            cx={4}
            cy={moonRadius * tilt + 2}
            rx={moonRadius * 1.3}
            ry={moonRadius * tilt * 0.55}
            fill="url(#lune-shadow)"
          />

          {/* moon halo bloom */}
          <circle
            cx={0}
            cy={0}
            r={moonRadius + 20}
            fill={tone.rim}
            filter="url(#orbit-dot-glow)"
          />

          {/* moon body */}
          <g
            style={{ cursor: moonCursor }}
            onClick={(e) => {
              e.stopPropagation();
              onMoonClick?.();
            }}
            data-orbit-hit
          >
            <circle cx={0} cy={0} r={moonRadius} fill="url(#lune-sphere)" />
            <ellipse
              cx={-moonRadius * 0.25}
              cy={-moonRadius * 0.3}
              rx={moonRadius * 0.32}
              ry={moonRadius * 0.24}
              fill="url(#lune-specular)"
            />
            {/* craters — subtle */}
            <circle cx={-moonRadius * 0.1} cy={moonRadius * 0.08} r={moonRadius * 0.055} fill="rgba(72,64,108,0.18)" />
            <circle cx={moonRadius * 0.22} cy={-moonRadius * 0.14} r={moonRadius * 0.04} fill="rgba(72,64,108,0.14)" />
            <circle cx={moonRadius * 0.08} cy={moonRadius * 0.34} r={moonRadius * 0.048} fill="rgba(72,64,108,0.16)" />
            <circle cx={moonRadius * 0.34} cy={moonRadius * 0.26} r={moonRadius * 0.034} fill="rgba(72,64,108,0.12)" />

            {/* lunar phase shadow overlay */}
            <MoonPhaseShadow phase={moonPhase} radius={moonRadius} />
          </g>

          {/* front arcs */}
          {orbitPools.map(({ pool, radius }, i) => (
            <path
              key={`front-ring-${pool.id}`}
              d={arcPath(radius, radius * tilt, "front")}
              fill="none"
              stroke="rgba(134,125,193,0.42)"
              strokeWidth={activePoolIdx === i ? 1.7 : 1.3}
              opacity={(activePoolIdx === i ? 1 : 0.82) * visibleRingOpacity}
            />
          ))}

          {/* front dots */}
          {frontDots.map((dot) => (
            <AccountDot
              key={`front-${dot.poolIdx}-${dot.accountIdx}`}
              dot={dot}
              frameTime={frameTime}
              highlighted={
                hover?.poolIdx === dot.poolIdx && hover?.accountIdx === dot.accountIdx
              }
              onEnter={() => setHover({ poolIdx: dot.poolIdx, accountIdx: dot.accountIdx })}
              onLeave={() => setHover(null)}
              onClick={() => onAccountClick?.(dot.pool.id, dot.account.id)}
            />
          ))}

          {/* moonFace: overlay on the moon disc, below hover labels so labels stay on top */}
          {moonFace ? (
            <foreignObject
              x={-moonRadius * 0.85}
              y={-moonRadius * 0.85}
              width={moonRadius * 1.7}
              height={moonRadius * 1.7}
              clipPath="url(#moon-clip)"
              style={{ pointerEvents: "none" }}
            >
              <div
                style={{
                  width: "100%",
                  height: "100%",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                {moonFace}
              </div>
            </foreignObject>
          ) : null}

          {hoveredDot ? (
            <HoverFloatLabel
              dot={hoveredDot}
              color={HEALTH_COLOR[hoveredDot.account.health]}
              statusText={HEALTH_LABEL[hoveredDot.account.health]}
            />
          ) : null}
        </g>
      </svg>
    </div>
  );
}

function AccountDot({
  dot,
  frameTime,
  highlighted,
  onEnter,
  onLeave,
  onClick,
}: {
  dot: DotPosition;
  frameTime: number;
  highlighted: boolean;
  onEnter: () => void;
  onLeave: () => void;
  onClick: () => void;
}) {
  const color = HEALTH_COLOR[dot.account.health];
  const d = (dot.depth + 1) / 2;
  const scale = 0.66 + 0.42 * d;

  // breathing: slow opacity wave, unique per dot
  const t = frameTime / dot.breathPeriod;
  const breath = 0.5 + 0.5 * Math.sin(t * Math.PI * 2 + dot.breathPhase);
  const breathOpacity = 0.68 + 0.32 * breath;
  const depthOpacity = 0.44 + 0.56 * d;
  const baseOpacity = depthOpacity * breathOpacity;
  const opacity = highlighted ? 1 : baseOpacity;

  const r = 4.4 * scale;
  const haloR = r * (highlighted ? 3.4 : 2.2 + breath * 0.6);

  return (
    <g
      data-orbit-hit
      style={{ cursor: "pointer", opacity }}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
    >
      <circle cx={dot.x} cy={dot.y} r={20} fill="transparent" />
      <circle
        cx={dot.x}
        cy={dot.y}
        r={haloR}
        fill={color}
        opacity={highlighted ? 0.42 : 0.2 + breath * 0.14}
        filter="url(#orbit-dot-glow)"
      />
      <circle cx={dot.x} cy={dot.y} r={r} fill={color} />
      <circle cx={dot.x} cy={dot.y} r={r * 0.34} fill="#ffffff" opacity={0.82} />
    </g>
  );
}

function HoverFloatLabel({
  dot,
  color,
  statusText,
}: {
  dot: DotPosition;
  color: string;
  statusText: string;
}) {
  // place label on the side away from the moon, clamped to viewBox bounds
  const side = dot.x >= 0 ? 1 : -1;
  const rawLx = dot.x + side * 28;
  const rawLy = dot.y - 8;
  const lx = Math.max(-470, Math.min(470, rawLx));
  const ly = Math.max(-360, Math.min(360, rawLy));
  const anchor = side === 1 ? "start" : "end";

  return (
    <g style={{ pointerEvents: "none" }}>
      <line
        x1={dot.x + side * 9}
        y1={dot.y}
        x2={lx - side * 4}
        y2={ly + 4}
        stroke={color}
        strokeWidth={0.8}
        opacity={0.55}
      />
      <text
        x={lx}
        y={ly - 10}
        textAnchor={anchor}
        fill="rgba(134,125,193,0.85)"
        style={{ font: '600 9px "Geist Variable", system-ui', letterSpacing: "0.18em", textTransform: "uppercase" }}
      >
        {dot.pool.label}
      </text>
      <text
        x={lx}
        y={ly + 6}
        textAnchor={anchor}
        fill="#21283f"
        style={{ font: '500 14px "Geist Variable", system-ui' }}
      >
        {dot.account.label}
      </text>
      <text
        x={lx}
        y={ly + 22}
        textAnchor={anchor}
        fill={color}
        style={{ font: '400 11px "Geist Variable", system-ui' }}
      >
        · {statusText}
      </text>
    </g>
  );
}

function MoonPhaseShadow({ phase, radius }: { phase: number; radius: number }) {
  // phase 0 = new moon (fully dark), 0.5 = full moon (fully lit), 1 = new moon again
  // We render an elliptical shadow over a full moon disc.
  // For 0 < phase < 0.5 (waxing): shadow on the LEFT, receding.
  // For 0.5 < phase < 1 (waning): shadow on the RIGHT, advancing.
  if (Math.abs(phase - 0.5) < 0.02) return null; // full moon — no shadow

  const waxing = phase < 0.5;
  const illumination = waxing ? phase * 2 : (1 - phase) * 2; // 0..1

  // ellipse rx controls curvature of terminator
  const rx = radius * Math.abs(1 - 2 * illumination);
  const shadowSide = waxing ? -1 : 1; // waxing: dark on left
  const shadowFill = "rgba(30,26,60,0.58)";

  // If illumination < 0.5, shadow is larger than half disk; render as full disc minus lit ellipse
  // Simpler approach: draw the dark semicircle, then draw an ellipse that is either cut-out (add light) or added (add dark)
  if (illumination > 0.5) {
    // more than half lit: shadow is a crescent on one side.
    // We draw the dark semicircle then "erase" a central ellipse back to lit.
    // Using a solid warm-lit color (not url(#lune-sphere)) — the sphere gradient
    // is objectBoundingBox-scaled and would misalign its highlight on a narrow ellipse.
    return (
      <g clipPath="url(#moon-clip)" style={{ pointerEvents: "none" }}>
        <path
          d={semicirclePath(radius, shadowSide)}
          fill={shadowFill}
        />
        <ellipse cx={0} cy={0} rx={rx} ry={radius} fill="#eadfc9" opacity={0.96} />
      </g>
    );
  }
  // less than half lit: shadow covers more than half, = semicircle plus ellipse on the other side
  return (
    <g clipPath="url(#moon-clip)" style={{ pointerEvents: "none" }}>
      <path d={semicirclePath(radius, shadowSide)} fill={shadowFill} />
      <ellipse cx={0} cy={0} rx={rx} ry={radius} fill={shadowFill} />
    </g>
  );
}

function semicirclePath(r: number, side: -1 | 1): string {
  // draw a semicircle on the given side of the y-axis
  if (side === -1) {
    return `M 0 ${-r} A ${r} ${r} 0 0 0 0 ${r} Z`;
  }
  return `M 0 ${-r} A ${r} ${r} 0 0 1 0 ${r} Z`;
}
