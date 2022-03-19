import Box from "@material-ui/core/Box"
import Link from "@material-ui/core/Link"
import Typography from "@material-ui/core/Typography"
import React from "react"

export const LANGUAGE = {
  emptyDisplayName: "*",
}

const TargetCellName = (displayName: string, onSelect: () => void): JSX.Element => {
  return displayName ? (
    <Link onClick={onSelect}>{displayName}</Link>
  ) : (
    <Typography variant="caption">{LANGUAGE.emptyDisplayName}</Typography>
  )
}

export interface TargetCellProps {
  name: string
  type: string
  onSelect: () => void
}
export namespace TargetCellProps {
  /**
   * @throws Error if invalid
   */
  export const validate = (props: TargetCellProps): TargetCellProps => {
    const sanitizedName = props.name.trim()
    const sanitizedType = props.type.trim()

    if (!sanitizedType) {
      throw new Error(`invalid type: '${props.type}'`)
    }

    return {
      name: sanitizedName,
      type: sanitizedType,
      onSelect: props.onSelect,
    }
  }
}

export const TargetCell: React.FC<TargetCellProps> = (props) => {
  const { name, type, onSelect } = TargetCellProps.validate(props)

  return (
    <Box display="flex" flexDirection="column">
      {TargetCellName(name, onSelect)}

      <Typography color="textSecondary" variant="caption">
        {type}
      </Typography>
    </Box>
  )
}
