import { useMemo } from "react";
import qrcode from "qrcode-generator";
import CopyButton from "@/components/CopyButton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export default function QrCodeDialog({
  open,
  onOpenChange,
  baseUrl,
  token,
  title,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  baseUrl: string;
  token: string;
  title: string;
}) {
  const payload = `lune://connect?base_url=${encodeURIComponent(baseUrl)}&api_key=${encodeURIComponent(token)}`;

  const svgMarkup = useMemo(() => {
    const qr = qrcode(0, "M");
    qr.addData(payload);
    qr.make();
    return qr.createSvgTag({
      cellSize: 6,
      margin: 2,
      scalable: true,
    });
  }, [payload]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md rounded-[1.6rem] border border-white/75 bg-white/95 p-0 sm:max-w-md">
        <DialogHeader className="border-b border-moon-200/60 px-6 py-5">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            扫码后可直接带入地址与 Token。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5 px-6 py-6">
          <div className="mx-auto flex w-fit items-center justify-center rounded-[1.5rem] bg-white p-4 shadow-[0_24px_54px_-42px_rgba(33,40,63,0.28)]">
            <div
              className="size-[15rem]"
              dangerouslySetInnerHTML={{ __html: svgMarkup }}
            />
          </div>
          <div className="surface-outline space-y-3 px-4 py-4">
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
              payload
            </p>
            <p className="break-all text-xs leading-6 text-moon-600">{payload}</p>
            <CopyButton value={payload} label="复制链接" />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
