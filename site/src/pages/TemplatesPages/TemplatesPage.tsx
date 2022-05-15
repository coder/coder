import React from "react"
import useSWR from "swr"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { TemplatesTable } from "../../components/TemplatesTable/TemplatesTable"

export const Language = {
  title: "Templates",
  tableTitle: "All templates",
  nameLabel: "Name",
  emptyMessage: "No templates have been created yet",
  emptyDescription: "Run the following command to get started:",
  totalLabel: "total",
}

export const TemplatesPage: React.FC = () => {
  const { data: orgs, error: orgsError } = useSWR<TypesGen.Organization[], Error>("/api/v2/users/me/organizations")
  const { data: templates, error } = useSWR<TypesGen.Template[] | undefined, Error>(
    orgs ? `/api/v2/organizations/${orgs[0].id}/templates` : undefined,
  )
  const subTitle = templates ? `${templates.length} ${Language.totalLabel}` : undefined
  const hasError = orgsError || error

  return (
    <Stack spacing={4}>
      <Header title={Language.title} subTitle={subTitle} />
      <Margins>
        {error && <ErrorSummary error={error} />}
        {orgsError && <ErrorSummary error={orgsError} />}
        {!hasError && <TemplatesTable organizations={orgs} templates={templates} />}
      </Margins>
    </Stack>
  )
}
