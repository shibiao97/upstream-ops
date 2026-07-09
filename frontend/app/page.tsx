"use client"

import { KpiRow } from "@/components/monitor/kpi-row"
import { BalanceOverview } from "@/components/monitor/balance-overview"
import { MultiplierChanges } from "@/components/monitor/multiplier-changes"
import { RelayUsageCard } from "@/components/monitor/relay-usage-card"
import { ChannelCards } from "@/components/monitor/channel-cards"
import { BottomPanels } from "@/components/monitor/bottom-panels"
import { useAuth } from "@/lib/auth-context"

export default function Page() {
  const { isSuperAdmin } = useAuth()
  return (
    <>
      <KpiRow />

      <div className={`grid grid-cols-1 gap-3 ${isSuperAdmin ? "lg:grid-cols-3" : "lg:grid-cols-2"}`}>
        <div>
          <BalanceOverview />
        </div>
        {isSuperAdmin ? (
          <div>
            <RelayUsageCard />
          </div>
        ) : null}
        <div>
          <MultiplierChanges />
        </div>
      </div>

      <ChannelCards />

      <BottomPanels />
    </>
  )
}
