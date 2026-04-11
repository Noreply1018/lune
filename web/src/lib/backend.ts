import { getBackendToken, setBackendToken, logout } from "./auth";

async function backendRequest<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getBackendToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
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

/** Log in to backend engine and persist the session token. */
export async function backendLogin(
  username: string,
  password: string,
): Promise<void> {
  const data = await backendRequest<{
    data: string;
    message: string;
    success: boolean;
  }>("POST", "/api/user/login", { username, password });
  if (!data.success) {
    throw new Error(data.message || "登录失败");
  }
  // Backend engine returns an access token in data.data
  if (data.data) {
    setBackendToken(data.data);
  }
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
