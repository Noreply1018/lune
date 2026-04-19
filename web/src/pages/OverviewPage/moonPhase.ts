export function getMoonPhase(date: Date = new Date()): number {
  const synodic = 29.530588853;
  const epoch = Date.UTC(2000, 0, 6, 18, 14, 0);
  const days = (date.getTime() - epoch) / (1000 * 60 * 60 * 24);
  const phase = ((days % synodic) + synodic) % synodic;
  return phase / synodic;
}

export function getMoonPhaseName(phase: number): string {
  if (phase < 0.03 || phase >= 0.97) return "新月";
  if (phase < 0.22) return "娥眉月";
  if (phase < 0.28) return "上弦月";
  if (phase < 0.47) return "渐盈凸月";
  if (phase < 0.53) return "满月";
  if (phase < 0.72) return "渐亏凸月";
  if (phase < 0.78) return "下弦月";
  return "残月";
}
