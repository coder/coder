import Box from "@mui/material/Box"
import { useQuery } from "@tanstack/react-query"
import { getHealth } from "api/api"
import { Loader } from "components/Loader/Loader"
import { useTab } from "hooks"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { colors } from "theme/colors"
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined"
import ErrorOutline from "@mui/icons-material/ErrorOutline"
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter"

const sections = {
  derp: "DERP",
  access_url: "Access URL",
  websocket: "Websocket",
  database: "Database",
}

export default function HealthPage() {
  const tab = useTab("tab", "derp")
  const { data: healthStatus } = useQuery({
    queryKey: ["health"],
    queryFn: () => getHealth(),
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle("Health")}</title>
      </Helmet>
      {healthStatus ? (
        <Box
          sx={{
            display: "flex",
            alignItems: "start",
            height: "calc(100vh - 62px - 36px)",
            overflow: "hidden",
            // Remove padding added from dashboard layout (.siteContent)
            marginBottom: "-48px",
          }}
        >
          <Box
            sx={{
              width: (theme) => theme.spacing(32),
              flexShrink: 0,
              borderRight: (theme) => `1px solid ${theme.palette.divider}`,
              height: "100%",
            }}
          >
            <Box
              sx={{
                fontSize: 10,
                textTransform: "uppercase",
                fontWeight: 500,
                color: (theme) => theme.palette.text.secondary,
                padding: (theme) => theme.spacing(1.5, 3),
                letterSpacing: "0.5px",
              }}
            >
              Health checks
            </Box>
            <Box component="nav">
              {Object.entries(sections).map(([key, label]) => {
                const isActive = tab.value === key
                const isHealthy =
                  healthStatus.data[key as keyof typeof healthStatus.data]
                    .healthy

                return (
                  <Box
                    component="button"
                    key={key}
                    onClick={() => {
                      tab.set(key)
                    }}
                    sx={{
                      background: isActive ? colors.gray[13] : "none",
                      border: "none",
                      fontSize: 14,
                      width: "100%",
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      textAlign: "left",
                      height: 36,
                      padding: (theme) => theme.spacing(0, 3),
                      cursor: "pointer",
                      pointerEvents: isActive ? "none" : "auto",
                      color: (theme) =>
                        isActive
                          ? theme.palette.text.primary
                          : theme.palette.text.secondary,
                      "&:hover": {
                        background: (theme) => theme.palette.action.hover,
                        color: (theme) => theme.palette.text.primary,
                      },
                    }}
                  >
                    {isHealthy ? (
                      <CheckCircleOutlined
                        sx={{
                          width: 16,
                          height: 16,
                          color: (theme) => theme.palette.success.light,
                        }}
                      />
                    ) : (
                      <ErrorOutline
                        sx={{
                          width: 16,
                          height: 16,
                          color: (theme) => theme.palette.error.main,
                        }}
                      />
                    )}
                    {label}
                  </Box>
                )
              })}
            </Box>
          </Box>
          {/* 62px - navbar and 36px - the bottom bar */}
          <Box sx={{ height: "100%", overflowY: "auto", width: "100%" }}>
            <SyntaxHighlighter
              language="json"
              editorProps={{ height: "100%" }}
              value={JSON.stringify(
                healthStatus.data[tab.value as keyof typeof healthStatus.data],
                null,
                2,
              )}
            />
          </Box>
        </Box>
      ) : (
        <Loader />
      )}
    </>
  )
}
