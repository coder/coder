import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useSearchParams } from "react-router-dom";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { pageTitle } from "utils/page";
import { paginatedAudits } from "api/queries/audits";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { isNonInitialPage } from "components/PaginationWidget/utils";
import { useFilter } from "components/Filter/filter";
import { useActionFilterMenu, useResourceTypeFilterMenu } from "./AuditFilter";
import { AuditPageView } from "./AuditPageView";

const AuditPage: FC = () => {
  const { audit_log: isAuditLogVisible } = useFeatureVisibility();

  /**
   * There is an implicit link between auditsQuery and filter via the
   * searchParams object
   *
   * @todo Make link more explicit (probably by making it so that components
   * and hooks can share the result of useSearchParams directly)
   */
  const [searchParams, setSearchParams] = useSearchParams();
  const auditsQuery = usePaginatedQuery(paginatedAudits(searchParams));
  const filter = useFilter({
    searchParamsResult: [searchParams, setSearchParams],
    onUpdate: auditsQuery.goToFirstPage,
  });

  const userMenu = useUserFilterMenu({
    value: filter.values.username,
    onChange: (option) =>
      filter.update({
        ...filter.values,
        username: option?.value,
      }),
  });

  const actionMenu = useActionFilterMenu({
    value: filter.values.action,
    onChange: (option) =>
      filter.update({
        ...filter.values,
        action: option?.value,
      }),
  });

  const resourceTypeMenu = useResourceTypeFilterMenu({
    value: filter.values["resource_type"],
    onChange: (option) =>
      filter.update({
        ...filter.values,
        resource_type: option?.value,
      }),
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle("Audit")}</title>
      </Helmet>

      <AuditPageView
        auditLogs={auditsQuery.data?.audit_logs}
        isNonInitialPage={isNonInitialPage(searchParams)}
        isAuditLogVisible={isAuditLogVisible}
        auditsQuery={auditsQuery}
        error={auditsQuery.error}
        filterProps={{
          filter,
          error: auditsQuery.error,
          menus: {
            user: userMenu,
            action: actionMenu,
            resourceType: resourceTypeMenu,
          },
        }}
      />
    </>
  );
};

export default AuditPage;
