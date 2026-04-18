import { Fragment, useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { latency, shortDate } from "@/lib/fmt";
import type { NotificationDelivery, NotificationEventType } from "@/lib/types";
import { cn } from "@/lib/utils";

type StatusFilter = "all" | "success" | "failed" | "dropped" | "test";

// Capped at the backend's hard limit (ListNotificationDeliveries clamps
// anything over 200). Notification volume is small — a handful per day in
// normal operation plus auto-prune — so 200 easily covers the visible
// history. If the list ever hits this cap, the `truncated` banner tells
// the user that older rows exist but are not in view.
const FETCH_LIMIT = 200;
// Aligned with Request Logs in ActivityPage: 5 rows per page keeps an
// expanded detail row + its neighbours inside one viewport.
const PAGE_SIZE = 5;

export default function NotificationHistorySection() {
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([]);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [filterEvent, setFilterEvent] = useState("all");
  const [filterStatus, setFilterStatus] = useState<StatusFilter>("all");
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);

  useEffect(() => {
    api.get<NotificationEventType[]>("/notifications/event-types")
      .then((data) => setEventTypes(data ?? []))
      .catch(() => {
        // Event types are optional — filter just falls back to "all".
      });
  }, []);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    api.get<NotificationDelivery[]>(
      `/notifications/deliveries?limit=${FETCH_LIMIT}`,
    )
      .then((items) => {
        if (cancelled) return;
        const list = items ?? [];
        setDeliveries(list);
        setTruncated(list.length >= FETCH_LIMIT);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "通知历史加载失败");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Reset to page 1 whenever filter flips — otherwise a filter change that
  // shrinks the list to one page leaves currentPage pointing at page 5 of 1
  // until clamping eventually catches up.
  useEffect(() => {
    setCurrentPage(1);
  }, [filterEvent, filterStatus]);

  const filteredDeliveries = useMemo(() => {
    return deliveries.filter((item) => {
      if (filterEvent !== "all" && item.event !== filterEvent) return false;
      if (filterStatus !== "all") {
        if (filterStatus === "test") {
          if (item.triggered_by !== "test") return false;
        } else {
          // Test-triggered rows get the "test" label in the table regardless
          // of their raw status, so treat them as a separate bucket and
          // exclude them from the success/failed/dropped filters to avoid
          // surfacing rows whose visible label contradicts the active
          // filter.
          if (item.triggered_by === "test") return false;
          if (item.status !== filterStatus) return false;
        }
      }
      return true;
    });
  }, [deliveries, filterEvent, filterStatus]);

  const totalPages = Math.max(
    1,
    Math.ceil(filteredDeliveries.length / PAGE_SIZE),
  );
  // Clamp at read time in case the filtered set shrinks under the current
  // page index (e.g. filter flip between renders).
  const clampedPage = Math.min(Math.max(1, currentPage), totalPages);
  const pagedDeliveries = useMemo(
    () =>
      filteredDeliveries.slice(
        (clampedPage - 1) * PAGE_SIZE,
        clampedPage * PAGE_SIZE,
      ),
    [filteredDeliveries, clampedPage],
  );

  const events = eventTypes.map((item) => item.event);

  return (
    <section
      id="notification-history"
      className="surface-section scroll-mt-6 px-5 py-5 sm:px-6"
    >
      <SectionHeading
        title="Notification History"
        description="查看每次渠道投递是成功、失败还是被人工测试触发。"
      />

      <div className="mt-5 flex flex-wrap gap-3">
        <select
          value={filterEvent}
          onChange={(event) => setFilterEvent(event.target.value)}
          className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
        >
          <option value="all">全部事件</option>
          {events.map((event) => (
            <option key={event} value={event}>
              {event}
            </option>
          ))}
        </select>
        <select
          value={filterStatus}
          onChange={(event) => setFilterStatus(event.target.value as StatusFilter)}
          className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
        >
          <option value="all">全部状态</option>
          <option value="success">success</option>
          <option value="failed">failed</option>
          <option value="dropped">dropped</option>
          <option value="test">test</option>
        </select>
      </div>

      {error ? (
        <p className="mt-4 text-sm text-status-red">{error}</p>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-moon-400">
        <p>
          匹配到 {filteredDeliveries.length.toLocaleString()} 条记录
          {filteredDeliveries.length > PAGE_SIZE
            ? `，每页 ${PAGE_SIZE} 条`
            : ""}
          。
        </p>
        {truncated ? (
          <p>
            为保证加载稳定，当前仅展示最近 {FETCH_LIMIT.toLocaleString()} 条。
          </p>
        ) : null}
      </div>

      <div className="mt-4 overflow-x-auto rounded-[1.45rem] border border-moon-200/60">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-moon-100/60 text-xs uppercase tracking-[0.16em] text-moon-400">
            <tr>
              <th className="px-4 py-3">Time</th>
              <th className="px-4 py-3">Event</th>
              <th className="px-4 py-3">Severity</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3">Upstream</th>
              <th className="px-4 py-3">Latency</th>
              <th className="px-4 py-3">Payload</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y divide-moon-200/50 bg-white/66">
            {pagedDeliveries.length ? pagedDeliveries.map((item) => {
              const expanded = expandedId === item.id;
              return (
                <Fragment key={item.id}>
                  <tr
                    className={cn(
                      "cursor-pointer transition-colors hover:bg-white/55",
                      expanded ? "bg-white/82" : "",
                    )}
                    onClick={() =>
                      setExpandedId((current) => (current === item.id ? null : item.id))
                    }
                  >
                    <td className="px-4 py-3 text-moon-400">{shortDate(item.created_at)}</td>
                    <td className="px-4 py-3 text-moon-500">{item.event}</td>
                    <td className="px-4 py-3 text-moon-500">{item.severity}</td>
                    <td className="px-4 py-3">
                      <span
                        className={cn(
                          "inline-flex rounded-full px-2 py-1 text-xs",
                          item.status === "success"
                            ? "bg-status-green/12 text-status-green"
                            : item.status === "failed"
                              ? "bg-status-red/12 text-status-red"
                              : "bg-moon-100/90 text-moon-500",
                        )}
                      >
                        {item.triggered_by === "test" ? "test" : item.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-moon-500">{item.upstream_code || "--"}</td>
                    <td className="px-4 py-3 text-moon-500">{latency(item.latency_ms)}</td>
                    <td className="max-w-xs px-4 py-3 text-moon-500">
                      <div className="truncate">{item.payload_summary || "--"}</div>
                    </td>
                    <td className="px-4 py-3 text-moon-300">
                      <ChevronDown
                        className={cn("size-4 transition-transform", expanded ? "rotate-180" : "")}
                      />
                    </td>
                  </tr>
                  {expanded ? (
                    <tr className="bg-white/80">
                      <td colSpan={8} className="px-4 py-4">
                        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                          <div className="space-y-1">
                            <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                              Title
                            </p>
                            <p className="text-sm text-moon-700">{item.title || "--"}</p>
                          </div>
                          <div className="space-y-1">
                            <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                              Attempt
                            </p>
                            <p className="text-sm text-moon-700">#{item.attempt}</p>
                          </div>
                          <div className="space-y-1">
                            <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                              Triggered By
                            </p>
                            <p className="text-sm text-moon-700">{item.triggered_by}</p>
                          </div>
                          <div className="space-y-1">
                            <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                              Upstream Message
                            </p>
                            <p className="text-sm text-moon-700">
                              {item.upstream_message || "--"}
                            </p>
                          </div>
                        </div>
                        <div className="mt-4 rounded-[1.1rem] border border-moon-200/45 bg-moon-50/80 px-4 py-3">
                          <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                            Payload Summary
                          </p>
                          <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-moon-600">
                            {item.payload_summary || "没有可展示的摘要。"}
                          </p>
                        </div>
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              );
            }) : (
              <tr>
                <td colSpan={8} className="px-4 py-12 text-center text-sm text-moon-400">
                  当前筛选下没有可显示的通知投递记录。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {totalPages > 1 ? (
        <div className="mt-4 flex items-center justify-between gap-3 text-sm text-moon-500">
          <span className="text-xs text-moon-400">
            第 {clampedPage} / {totalPages} 页
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              disabled={clampedPage <= 1}
              onClick={() => setCurrentPage(Math.max(1, clampedPage - 1))}
            >
              <ChevronLeft className="size-4" />
              上一页
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              disabled={clampedPage >= totalPages}
              onClick={() =>
                setCurrentPage(Math.min(totalPages, clampedPage + 1))
              }
            >
              下一页
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
