import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type Column<T> = {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  className?: string;
  align?: "left" | "center" | "right";
  tone?: "primary" | "secondary" | "status" | "numeric";
};

export default function DataTable<T>({
  columns,
  rows,
  rowKey,
  empty,
  tableClassName,
}: {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string | number;
  empty?: string;
  tableClassName?: string;
}) {
  if (rows.length === 0) {
    return (
      <p className="py-12 text-center text-sm text-moon-400">
        {empty ?? "No data"}
      </p>
    );
  }
  return (
    <Table className={tableClassName}>
      <TableHeader>
        <TableRow>
          {columns.map((col) => (
            <TableHead
              key={col.key}
              className={cn(
                "h-11 px-4 text-[11px] font-semibold uppercase tracking-[0.2em] text-moon-400",
                col.align === "right" && "text-right",
                col.align === "center" && "text-center",
                col.className,
              )}
            >
              {col.header}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow
            key={rowKey(row)}
            className="border-moon-200/55 transition-colors hover:bg-moon-100/40"
          >
            {columns.map((col) => (
              <TableCell
                key={col.key}
                className={cn(
                  "px-4 py-3.5 align-middle",
                  col.align === "right" && "text-right",
                  col.align === "center" && "text-center",
                  col.tone === "primary" && "font-medium text-moon-800",
                  col.tone === "secondary" && "text-moon-500",
                  col.tone === "numeric" &&
                    "font-medium tabular-nums text-moon-700",
                  col.tone === "status" && "text-moon-700",
                  col.className,
                )}
              >
                {col.render(row)}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
