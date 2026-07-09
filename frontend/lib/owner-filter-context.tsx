"use client"

import { createContext, useContext, useMemo, useState, type ReactNode } from "react"

export type OwnerFilter = number | "all"

interface OwnerFilterContextValue {
  ownerFilter: OwnerFilter
  setOwnerFilter: (v: OwnerFilter) => void
}

const OwnerFilterContext = createContext<OwnerFilterContextValue | null>(null)

export function OwnerFilterProvider({ children }: { children: ReactNode }) {
  const [ownerFilter, setOwnerFilter] = useState<OwnerFilter>("all")
  const value = useMemo(() => ({ ownerFilter, setOwnerFilter }), [ownerFilter])
  return <OwnerFilterContext.Provider value={value}>{children}</OwnerFilterContext.Provider>
}

export function useOwnerFilter() {
  const ctx = useContext(OwnerFilterContext)
  if (!ctx) return { ownerFilter: "all" as OwnerFilter, setOwnerFilter: () => {} }
  return ctx
}
