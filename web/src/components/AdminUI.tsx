import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { PoolSnapshot } from "@/lib/lune";

type AdminUIContextValue = {
  addAccountOpen: boolean;
  preferredPoolId: number | null;
  dataVersion: number;
  poolSnapshots: Record<number, PoolSnapshot>;
  openAddAccount: (poolId?: number | null) => void;
  closeAddAccount: () => void;
  refreshData: () => void;
  setPoolSnapshots: (snapshots: Record<number, PoolSnapshot>) => void;
};

const AdminUIContext = createContext<AdminUIContextValue | null>(null);

export function AdminUIProvider({ children }: { children: ReactNode }) {
  const [addAccountOpen, setAddAccountOpen] = useState(false);
  const [preferredPoolId, setPreferredPoolId] = useState<number | null>(null);
  const [dataVersion, setDataVersion] = useState(0);
  const [poolSnapshots, setPoolSnapshots] = useState<Record<number, PoolSnapshot>>({});

  const openAddAccount = useCallback((poolId?: number | null) => {
    setPreferredPoolId(poolId ?? null);
    setAddAccountOpen(true);
  }, []);

  const closeAddAccount = useCallback(() => {
    setAddAccountOpen(false);
  }, []);

  const refreshData = useCallback(() => {
    setDataVersion((current) => current + 1);
  }, []);

  const value = useMemo<AdminUIContextValue>(
    () => ({
      addAccountOpen,
      preferredPoolId,
      dataVersion,
      poolSnapshots,
      openAddAccount,
      closeAddAccount,
      refreshData,
      setPoolSnapshots,
    }),
    [addAccountOpen, closeAddAccount, dataVersion, openAddAccount, poolSnapshots, preferredPoolId, refreshData],
  );

  return (
    <AdminUIContext.Provider value={value}>{children}</AdminUIContext.Provider>
  );
}

export function useAdminUI() {
  const value = useContext(AdminUIContext);
  if (!value) {
    throw new Error("useAdminUI must be used within AdminUIProvider.");
  }
  return value;
}
