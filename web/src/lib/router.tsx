import {
  createContext,
  useContext,
  useEffect,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react";

function normalizePath(pathname: string): string {
  const normalized = pathname.replace(/\/$/, "");
  return normalized || "/admin";
}

type RouterContextValue = {
  pathname: string;
  navigate: (href: string, options?: { replace?: boolean }) => void;
  onLinkClick: (
    event: MouseEvent<HTMLAnchorElement>,
    href: string,
    options?: { replace?: boolean },
  ) => void;
};

const RouterContext = createContext<RouterContextValue | null>(null);

export function RouterProvider({ children }: { children: ReactNode }) {
  const [pathname, setPathname] = useState(() =>
    normalizePath(window.location.pathname),
  );

  useEffect(() => {
    const handlePopState = () => {
      setPathname(normalizePath(window.location.pathname));
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const value: RouterContextValue = {
    pathname,
    navigate: (href, options) => {
      const next = normalizePath(href);
      if (next === pathname) return;

      const method = options?.replace ? "replaceState" : "pushState";
      window.history[method](null, "", href);
      setPathname(next);
    },
    onLinkClick: (event, href, options) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }

      const target = event.currentTarget.target;
      if (target && target !== "_self") {
        return;
      }

      const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
      if (current === href) {
        event.preventDefault();
        return;
      }

      event.preventDefault();
      window.history[options?.replace ? "replaceState" : "pushState"](
        null,
        "",
        href,
      );
      setPathname(normalizePath(href));
    },
  };

  return (
    <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
  );
}

export function useRouter() {
  const value = useContext(RouterContext);
  if (!value) {
    throw new Error("useRouter must be used within a RouterProvider.");
  }
  return value;
}

export function usePathname() {
  return useRouter().pathname;
}

export function matchPath(
  pattern: string,
  pathname: string,
): Record<string, string> | null {
  const patternParts = normalizePath(pattern).split("/");
  const pathParts = normalizePath(pathname).split("/");

  if (patternParts.length !== pathParts.length) {
    return null;
  }

  const params: Record<string, string> = {};

  for (let i = 0; i < patternParts.length; i += 1) {
    const expected = patternParts[i];
    const actual = pathParts[i];

    if (expected.startsWith(":")) {
      params[expected.slice(1)] = decodeURIComponent(actual);
      continue;
    }

    if (expected !== actual) {
      return null;
    }
  }

  return params;
}
