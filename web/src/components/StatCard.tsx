export default function StatCard({
  label,
  value,
  sub,
}: {
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="rounded-xl border border-paper-200 bg-paper-100 px-5 py-4">
      <p className="text-xs text-paper-500 tracking-wide">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-paper-800">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-paper-300">{sub}</p>}
    </div>
  );
}
