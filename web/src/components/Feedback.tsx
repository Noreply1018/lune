import { toast as sonnerToast } from "sonner";

type FeedbackType = "success" | "error";

export function toast(msg: string, type: FeedbackType = "success") {
  if (type === "error") sonnerToast.error(msg);
  else sonnerToast.success(msg);
}
