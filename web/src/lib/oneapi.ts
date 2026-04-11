import { getOneapiToken, setOneapiToken, logout } from "./auth";

async function oneapiRequest<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getOneapiToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(`/oneapi${path}`, {
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

/** Log in to One-API and persist the session token. */
export async function oneapiLogin(
  username: string,
  password: string,
): Promise<void> {
  const data = await oneapiRequest<{
    data: string;
    message: string;
    success: boolean;
  }>("POST", "/api/user/login", { username, password });
  if (!data.success) {
    throw new Error(data.message || "One-API login failed");
  }
  // One-API returns an access token in data.data
  if (data.data) {
    setOneapiToken(data.data);
  }
}

export function oneapiGet<T>(path: string) {
  return oneapiRequest<T>("GET", path);
}

export function oneapiPost<T>(path: string, body?: unknown) {
  return oneapiRequest<T>("POST", path, body);
}

export function oneapiPut<T>(path: string, body?: unknown) {
  return oneapiRequest<T>("PUT", path, body);
}

export function oneapiDelete<T>(path: string) {
  return oneapiRequest<T>("DELETE", path);
}
