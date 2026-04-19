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

export default function MoonInscription({
  requests,
  successRate,
  avgLatency,
}: {
  requests: number;
  successRate: number;
  avgLatency: number;
}) {
  const successPct = (successRate * 100).toFixed(1);
  const latencyText = avgLatency >= 1000 ? `${(avgLatency / 1000).toFixed(1)}` : `${Math.round(avgLatency)}`;
  const latencyUnit = avgLatency >= 1000 ? "秒" : "毫秒";

  return (
    <div
      className="flex flex-col items-center justify-center gap-2 text-moon-700"
      style={{
        fontFamily: "'Iowan Old Style','Palatino Linotype','Noto Serif SC','Source Han Serif SC',Georgia,serif",
        lineHeight: 1.3,
        textAlign: "center",
      }}
    >
      <Line big={toChineseDigits(requests)} small="次请求" />
      <div className="h-px w-6 bg-moon-400/35" />
      <Line big={toChineseDigits(successPct)} small="成功率 %" />
      <div className="h-px w-6 bg-moon-400/35" />
      <Line big={toChineseDigits(latencyText)} small={latencyUnit} />
    </div>
  );
}

function Line({ big, small }: { big: string; small: string }) {
  return (
    <div className="flex flex-col items-center">
      <span
        className="whitespace-nowrap text-[13px] font-medium tracking-[0.24em]"
        style={{ color: "rgba(33,40,63,0.78)" }}
      >
        {big}
      </span>
      <span className="whitespace-nowrap text-[8px] tracking-[0.32em] text-moon-500" style={{ marginTop: 1 }}>
        {small}
      </span>
    </div>
  );
}
