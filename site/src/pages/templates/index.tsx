import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Link } from "react-router-dom"
import useSWR from "swr"
import { Organization, Template } from "../../api/types"
import { EmptyState } from "../../components"
import { CodeExample } from "../../components/CodeExample/CodeExample"
import { ErrorSummary } from "../../components/ErrorSummary"
import { Header } from "../../components/Header"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Footer } from "../../components/Page"
import { Column, Table } from "../../components/Table"

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
    <div className={styles.root}>
      <Header title="Templates" subTitle={subTitle} />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Table {...tableProps} />
      </Paper>
      <Footer />
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
  descriptionLabel: {
    marginBottom: theme.spacing(1),
  },
}))
