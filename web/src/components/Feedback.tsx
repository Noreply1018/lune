import { useEffect, useState } from "react";

type FeedbackType = "success" | "error";

let showFn: ((msg: string, type: FeedbackType) => void) | null = null;

/** Show a toast message from anywhere. */
export function toast(msg: string, type: FeedbackType = "success") {
  showFn?.(msg, type);
}

export default function Feedback() {
  const [msg, setMsg] = useState("");
  const [type, setType] = useState<FeedbackType>("success");
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    showFn = (m, t) => {
      setMsg(m);
      setType(t);
      setVisible(true);
      setTimeout(() => setVisible(false), 3000);
    };
    return () => {
      showFn = null;
    };
  }, []);

  if (!visible) return null;

  const bg =
    type === "success"
      ? "bg-sage-500/15 text-sage-600 border-sage-500/30"
      : "bg-clay-500/15 text-clay-600 border-clay-500/30";

  return (
    <div
      className={`fixed top-4 right-4 z-50 rounded-lg border px-4 py-2.5 text-sm shadow-sm ${bg}`}
    >
      {msg}
    </div>
  );
}
