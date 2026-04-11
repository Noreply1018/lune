const styles: Record<string, string> = {
  ok: "bg-sage-500/15 text-sage-600",
  error: "bg-clay-500/15 text-clay-600",
  disabled: "bg-paper-200 text-paper-500",
};

export default function StatusBadge({
  status,
  label,
}: {
  status: "ok" | "error" | "disabled";
  label?: string;
}) {
  return (
    <span
      className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${styles[status] ?? styles.disabled}`}
    >
      {label ?? status}
    </span>
  );
}
