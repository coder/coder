import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Link } from "react-router-dom"
import useSWR from "swr"
import { Organization, Template } from "../../api/types"
import { CodeExample } from "../../components/CodeExample/CodeExample"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { Column, Table } from "../../components/Table/Table"

export const TemplatesPage: React.FC = () => {
  const styles = useStyles()
  const { data: orgs, error: orgsError } = useSWR<Organization[], Error>("/api/v2/users/me/organizations")
  const { data: templates, error } = useSWR<Template[] | null, Error>(
    orgs ? `/api/v2/organizations/${orgs[0].id}/templates` : null,
  )

  if (error) {
    return <ErrorSummary error={error} />
  }

  if (orgsError) {
    return <ErrorSummary error={error} />
  }

  if (!templates || !orgs) {
    return <FullScreenLoader />
  }

  // Create a dictionary of organization ID -> organization Name
  // Needed to properly construct links to dive into a template
  const orgDictionary = orgs.reduce((acc: Record<string, string>, curr: Organization) => {
    return {
      ...acc,
      [curr.id]: curr.name,
    }
  }, {})

  const columns: Column<Template>[] = [
    {
      key: "name",
      name: "Name",
      renderer: (nameField: string, data: Template) => {
        return <Link to={`/templates/${orgDictionary[data.organization_id]}/${nameField}`}>{nameField}</Link>
      },
    },
  ]

  const description = (
    <div>
      <div className={styles.descriptionLabel}>Run the following command to get started:</div>
      <CodeExample code="coder templates create" />
    </div>
  )

  const emptyState = <EmptyState message="No templates have been created yet" description={description} />

  const tableProps = {
    title: "All Templates",
    columns: columns,
    emptyState: emptyState,
    data: templates,
  }

  const subTitle = `${templates.length} total`

  return (
    <Stack spacing={4}>
      <Header title="Templates" subTitle={subTitle} />
      <Margins>
        <Table {...tableProps} />
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  descriptionLabel: {
    marginBottom: theme.spacing(1),
  },
}))
