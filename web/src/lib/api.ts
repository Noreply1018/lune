async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `${res.status} ${res.statusText}`);
  }
  return res.json();
}

export const luneGet = <T>(path: string) => request<T>("GET", path);
export const lunePost = <T>(path: string, body?: unknown) =>
  request<T>("POST", path, body);
export const lunePut = <T>(path: string, body?: unknown) =>
  request<T>("PUT", path, body);
export const luneDelete = <T>(path: string) => request<T>("DELETE", path);
