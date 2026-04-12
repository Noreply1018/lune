import { logout } from "./auth";

async function backendRequest<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(`/backend${path}`, {
    method,
    headers,
    credentials: "include",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    logout();
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `${res.status} ${res.statusText}`);
  }
  return res.json();
}

export function backendGet<T>(path: string) {
  return backendRequest<T>("GET", path);
}

export function backendPost<T>(path: string, body?: unknown) {
  return backendRequest<T>("POST", path, body);
}

export function backendPut<T>(path: string, body?: unknown) {
  return backendRequest<T>("PUT", path, body);
}

export function backendDelete<T>(path: string) {
  return backendRequest<T>("DELETE", path);
}
