import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import { useRouter } from "next/router"
import Link from "next/link"
import { EmptyState } from "../../components"
import { ErrorSummary } from "../../components/ErrorSummary"
import { Navbar } from "../../components/Navbar"
import { Header } from "../../components/Header"
import { Footer } from "../../components/Page"
import { Column, Table } from "../../components/Table"
import { useUser } from "../../contexts/UserContext"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"

import { Organization, Project } from "./../../api"
import useSWR from "swr"

const ProjectsPage: React.FC = () => {
  const styles = useStyles()
  const router = useRouter()
  const { me, signOut } = useUser(true)
  const { data, error } = useSWR<Project[] | null, Error>("/api/v2/projects")
  const { data: orgs, error: orgsError } = useSWR<Organization[], Error>("/api/v2/users/me/organizations")

  // TODO: The API call is currently returning `null`, which isn't ideal
  // - it breaks checking for data presence with SWR.
  const projects = data || []

  if (error) {
    return <ErrorSummary error={error} />
  }

  if (orgsError) {
    return <ErrorSummary error={error} />
  }

  if (!me || !projects || !orgs) {
    return <FullScreenLoader />
  }

  const createProject = () => {
    void router.push("/projects/create")
  }

  const action = {
    text: "Create Project",
    onClick: createProject,
  }

  // Create a dictionary of organization ID -> organization Name
  // Needed to properly construct links to dive into a project
  const orgDictionary = orgs.reduce((acc: Record<string, string>, curr: Organization) => {
    return {
      ...acc,
      [curr.id]: curr.name,
    }
  }, {})

  const columns: Column<Project>[] = [
    {
      key: "name",
      name: "Name",
      renderer: (nameField: string, data: Project) => {
        return <Link href={`/projects/${orgDictionary[data.organization_id]}/${nameField}`}>{nameField}</Link>
      },
    },
  ]

  const emptyState = (
    <EmptyState
      button={{
        children: "Create Project",
        onClick: createProject,
      }}
      message="No projects have been created yet"
      description="Create a project to get started."
    />
  )

  const tableProps = {
    title: "All Projects",
    columns: columns,
    emptyState: emptyState,
    data: projects,
  }

  const subTitle = `${projects.length} total`

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />
      <Header title="Projects" subTitle={subTitle} action={action} />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Table {...tableProps} />
      </Paper>
      <Footer />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
}))

export default ProjectsPage
