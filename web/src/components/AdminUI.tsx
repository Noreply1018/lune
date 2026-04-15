import {
  createContext,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type AdminUIContextValue = {
  addAccountOpen: boolean;
  preferredPoolId: number | null;
  dataVersion: number;
  openAddAccount: (poolId?: number | null) => void;
  closeAddAccount: () => void;
  refreshData: () => void;
};

const AdminUIContext = createContext<AdminUIContextValue | null>(null);

export function AdminUIProvider({ children }: { children: ReactNode }) {
  const [addAccountOpen, setAddAccountOpen] = useState(false);
  const [preferredPoolId, setPreferredPoolId] = useState<number | null>(null);
  const [dataVersion, setDataVersion] = useState(0);

  const value = useMemo<AdminUIContextValue>(
    () => ({
      addAccountOpen,
      preferredPoolId,
      dataVersion,
      openAddAccount: (poolId) => {
        setPreferredPoolId(poolId ?? null);
        setAddAccountOpen(true);
      },
      closeAddAccount: () => {
        setAddAccountOpen(false);
      },
      refreshData: () => {
        setDataVersion((current) => current + 1);
      },
    }),
    [addAccountOpen, dataVersion, preferredPoolId],
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
