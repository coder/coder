import { DashboardLayout } from "components/Dashboard/DashboardLayout"
import { RequireAuth } from "components/RequireAuth/RequireAuth"
import { FC } from "react"
import { createMemoryRouter, RouterProvider } from "react-router-dom"

export const DashboardPage: FC<{
  path: string
  page: JSX.Element
  layout?: JSX.Element
}> = ({ path, page, layout }) => {
  const router = createMemoryRouter(
    [
      {
        element: <RequireAuth />,
        children: [
          {
            element: <DashboardLayout />,
            children: [
              {
                path,
                element: layout ? layout : page,
                children: layout ? [{ index: true, element: page }] : undefined,
              },
            ],
          },
        ],
      },
    ],
    {
      initialEntries: [path],
    },
  )

  return <RouterProvider router={router} />
}
