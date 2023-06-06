import { UserFilterMenu, UserMenu } from "components/Filter/UserFilter"
import {
  Filter,
  MenuSkeleton,
  SearchFieldSkeleton,
  useFilter,
} from "components/Filter/filter"

export const AuditFilter = ({
  filter,
  error,
  menus,
}: {
  filter: ReturnType<typeof useFilter>
  error?: unknown
  menus: {
    user: UserFilterMenu
  }
}) => {
  return (
    <Filter
      isLoading={menus.user.isInitializing}
      filter={filter}
      error={error}
      options={<UserMenu {...menus.user} />}
      skeleton={
        <>
          <SearchFieldSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  )
}
