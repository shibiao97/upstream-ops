import { KpiRow } from "@/components/monitor/kpi-row"
import { BalanceOverview } from "@/components/monitor/balance-overview"
import { MultiplierChanges } from "@/components/monitor/multiplier-changes"
import { RelayUsageCard } from "@/components/monitor/relay-usage-card"
import { ChannelCards } from "@/components/monitor/channel-cards"
import { BottomPanels } from "@/components/monitor/bottom-panels"

export default function Page() {
  return (
    <>
      <KpiRow />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
        <div>
          <BalanceOverview />
        </div>
        <div>
          <RelayUsageCard />
        </div>
        <div>
          <MultiplierChanges />
        </div>
      </div>

      <ChannelCards />

      <BottomPanels />
    </>
  )
}
