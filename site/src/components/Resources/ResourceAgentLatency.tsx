import { makeStyles } from "@material-ui/core/styles"
import StarIcon from "@material-ui/icons/Star"
import React from "react"
import { WorkspaceAgent } from "../../api/typesGenerated"
import { HelpTooltip, HelpTooltipText } from "../Tooltips/HelpTooltip"

export interface ResourceAgentLatencyProps {
  latency: WorkspaceAgent["latency"]
}

export const ResourceAgentLatency: React.FC<ResourceAgentLatencyProps> = (props) => {
  const styles = useStyles()
  if (!props.latency) {
    return null
  }
  if (Object.keys(props.latency).length === 0) {
    return null
  }
  const latency = props.latency
  return (
    <div className={styles.root}>
      <div className={styles.title}>
        <b>Latency</b>
        <HelpTooltip size="small">
          <HelpTooltipText>
            Latency from relay servers, used when connections cannot connect peer-to-peer. Star
            indicates the preferred relay.
          </HelpTooltipText>
        </HelpTooltip>
      </div>
      {Object.keys(latency)
        .sort()
        .map((region) => {
          const value = latency[region]
          return (
            <div key={region} className={styles.region}>
              <b>{region}:</b>&nbsp;{Math.round(value.latency_ms * 100) / 100} ms
              {value.preferred && <StarIcon className={styles.star} />}
            </div>
          )
        })}
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    display: "grid",
    gap: 6,
  },
  title: {
    fontSize: "Something",
    display: "flex",
    alignItems: "center",
    // This ensures that the latency aligns with other columns in the grid.
    height: 20,
  },
  region: {
    display: "flex",
    alignItems: "center",
  },
  star: {
    width: 14,
    height: 14,
    marginLeft: 4,
  },
}))
