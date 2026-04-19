import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

const CN_DIGIT = ["〇", "一", "二", "三", "四", "五", "六", "七", "八", "九"];

function toChineseDigits(value: number | string): string {
  return String(value)
    .split("")
    .map((ch) => {
      if (ch === ".") return "点";
      if (ch === "-") return "负";
      if (ch >= "0" && ch <= "9") return CN_DIGIT[parseInt(ch, 10)];
      return ch;
    })
    .join("");
}

type MoonInscriptionProps = {
  requests: number;
  successRate: number;
  avgLatency: number;
};

export default function MoonInscription({
  requests,
  successRate,
  avgLatency,
}: MoonInscriptionProps) {
  const [awake, setAwake] = useState(false);
  const [open, setOpen] = useState(false);
  const awakeTimerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (awakeTimerRef.current) window.clearTimeout(awakeTimerRef.current);
    };
  }, []);

  function touch() {
    setAwake(true);
    if (awakeTimerRef.current) window.clearTimeout(awakeTimerRef.current);
    awakeTimerRef.current = window.setTimeout(() => setAwake(false), 2200);
  }

  const successPct = (successRate * 100).toFixed(1);
  const latencyText =
    avgLatency >= 1000 ? `${(avgLatency / 1000).toFixed(1)}` : `${Math.round(avgLatency)}`;
  const latencyUnit = avgLatency >= 1000 ? "秒" : "毫秒";

  return (
    <div
      onMouseEnter={() => {
        setAwake(true);
        if (awakeTimerRef.current) {
          window.clearTimeout(awakeTimerRef.current);
          awakeTimerRef.current = null;
        }
      }}
      onMouseLeave={() => {
        if (!open) touch();
      }}
      className={cn(
        "pointer-events-auto select-none transition-opacity duration-700",
        awake || open ? "opacity-100" : "opacity-40",
      )}
    >
      <button
        type="button"
        aria-expanded={open}
        aria-label="今日统计，点击展开详情"
        onClick={() => setOpen((v) => !v)}
        className="block text-left"
        style={{
          fontFamily:
            "'Iowan Old Style','Palatino Linotype','Noto Serif SC','Source Han Serif SC',Georgia,serif",
        }}
      >
        <InscriptionLine big={toChineseDigits(requests)} small="次请求" awake={awake || open} />
        <div className="my-1 h-px w-6 bg-moon-400/35" />
        <InscriptionLine big={toChineseDigits(successPct)} small="成功率 %" awake={awake || open} />
        <div className="my-1 h-px w-6 bg-moon-400/35" />
        <InscriptionLine big={toChineseDigits(latencyText)} small={latencyUnit} awake={awake || open} />
      </button>

      <div
        className={cn(
          "mt-2 grid transition-all duration-500 ease-out",
          open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
        )}
      >
        <div className="overflow-hidden">
          <div className="rounded-[1rem] border border-moon-200/55 bg-white/80 p-3 shadow-[0_18px_40px_-30px_rgba(33,40,63,0.4)] backdrop-blur-md">
            <DetailRow label="请求总数" value={requests.toLocaleString()} unit="次" />
            <div className="my-1.5 h-px bg-moon-200/45" />
            <DetailRow label="成功率" value={successPct} unit="%" />
            <div className="my-1.5 h-px bg-moon-200/45" />
            <DetailRow label="平均延迟" value={latencyText} unit={latencyUnit} />
          </div>
        </div>
      </div>
    </div>
  );
}

function InscriptionLine({
  big,
  small,
  awake,
}: {
  big: string;
  small: string;
  awake: boolean;
}) {
  return (
    <div className="flex flex-col items-start">
      <span
        className={cn(
          "whitespace-nowrap text-[13px] font-medium tracking-[0.24em] transition-colors duration-500",
          awake ? "text-moon-700" : "text-moon-500",
        )}
        style={{ color: awake ? "rgba(33,40,63,0.78)" : undefined }}
      >
        {big}
      </span>
      <span
        className="whitespace-nowrap text-[8px] tracking-[0.32em] text-moon-500"
        style={{ marginTop: 1 }}
      >
        {small}
      </span>
    </div>
  );
}

function DetailRow({ label, value, unit }: { label: string; value: string; unit: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-[10px] uppercase tracking-[0.28em] text-moon-400">{label}</span>
      <span className="font-mono text-[12px] text-moon-700">
        {value}
        <span className="ml-0.5 text-[9px] text-moon-500">{unit}</span>
      </span>
    </div>
  );
}
