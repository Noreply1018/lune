import { Fragment, useEffect, useRef, useState } from "react";
import { ChevronDown, RefreshCw } from "lucide-react";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { latency, shortDate } from "@/lib/fmt";
import type { NotificationDelivery, NotificationEventType } from "@/lib/types";
import { cn } from "@/lib/utils";

type StatusFilter = "all" | "success" | "failed" | "dropped" | "test";

const PAGE_SIZE = 40;

export default function NotificationHistorySection() {
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([]);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [filterEvent, setFilterEvent] = useState("all");
  const [filterStatus, setFilterStatus] = useState<StatusFilter>("all");
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestIdRef = useRef(0);

  useEffect(() => {
    api.get<NotificationEventType[]>("/notifications/event-types")
      .then((data) => setEventTypes(data ?? []))
      .catch(() => {
        // Event types are optional — filter just falls back to "all".
      });
  }, []);

  function load(reset = true) {
    const requestId = ++requestIdRef.current;
    const before = !reset && deliveries.length ? deliveries[deliveries.length - 1] : null;
    const params = new URLSearchParams();
    params.set("limit", String(PAGE_SIZE));
    if (filterEvent !== "all") params.set("event", filterEvent);
    if (filterStatus !== "all") {
      if (filterStatus === "test") params.set("triggered_by", "test");
      else params.set("status", filterStatus);
    }
    if (before) {
      params.set("before", before.created_at);
      params.set("before_id", String(before.id));
    }
    if (reset) setError(null);
    else setLoadingMore(true);

    api.get<NotificationDelivery[]>(`/notifications/deliveries?${params.toString()}`)
      .then((items) => {
        if (requestId !== requestIdRef.current) return;
        setDeliveries((current) => (reset ? (items ?? []) : [...current, ...(items ?? [])]));
        setHasMore((items?.length ?? 0) >= PAGE_SIZE);
      })
      .catch((err) => {
        if (requestId !== requestIdRef.current) return;
        setError(err instanceof Error ? err.message : "通知历史加载失败");
      })
      .finally(() => {
        if (requestId === requestIdRef.current) setLoadingMore(false);
      });
  }

  useEffect(() => {
    load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterEvent, filterStatus]);

  const events = eventTypes.map((item) => item.event);

  return (
    <section
      id="notification-history"
      className="surface-section scroll-mt-6 px-5 py-5"
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
            {deliveries.length ? deliveries.map((item) => {
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

      {hasMore ? (
        <div className="mt-4 flex justify-center">
          <Button variant="outline" onClick={() => load(false)} disabled={loadingMore}>
            {loadingMore ? <RefreshCw className="size-4 animate-spin" /> : null}
            Load More
          </Button>
        </div>
      ) : null}
    </section>
  );
}
