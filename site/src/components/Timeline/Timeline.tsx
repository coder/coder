import { TimelineDateRow } from "components/Timeline/TimelineDateRow"
import { Fragment } from "react"

type GetDateFn<TData> = (data: TData) => Date

const groupByDate = <TData,>(
  items: TData[],
  getDate: GetDateFn<TData>,
): Record<string, TData[]> => {
  const itemsByDate: Record<string, TData[]> = {}

  items.forEach((item) => {
    const dateKey = getDate(item).toDateString()

    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Is not a guarantee a value is defined when access it dynamically
    if (itemsByDate[dateKey]) {
      itemsByDate[dateKey].push(item)
    } else {
      itemsByDate[dateKey] = [item]
    }
  })

  return itemsByDate
}

export interface TimelineProps<TData> {
  items: TData[]
  getDate: GetDateFn<TData>
  row: (item: TData) => JSX.Element
}

export const Timeline = <TData,>({
  items,
  getDate,
  row,
}: TimelineProps<TData>): JSX.Element => {
  const itemsByDate = groupByDate(items, getDate)

  return (
    <>
      {Object.keys(itemsByDate).map((dateStr) => {
        const items = itemsByDate[dateStr]

        return (
          <Fragment key={dateStr}>
            <TimelineDateRow date={new Date(dateStr)} />
            {items.map(row)}
          </Fragment>
        )
      })}
    </>
  )
}
