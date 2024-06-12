import { useTheme } from "@emotion/react";
import KeyboardArrowDownOutlined from "@mui/icons-material/KeyboardArrowDownOutlined";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Skeleton from "@mui/material/Skeleton";
import { visuallyHidden } from "@mui/utils";
import { type FC, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { Abbr } from "components/Abbr/Abbr";
import { displayError } from "components/GlobalSnackbar/utils";
import { Latency } from "components/Latency/Latency";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { BUTTON_SM_HEIGHT } from "theme/constants";

interface ProxyMenuProps {
  proxyContextValue: ProxyContextValue;
}

export const ProxyMenu: FC<ProxyMenuProps> = ({ proxyContextValue }) => {
  const theme = useTheme();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [refetchDate, setRefetchDate] = useState<Date>();
  const selectedProxy = proxyContextValue.proxy.proxy;
  const refreshLatencies = proxyContextValue.refetchProxyLatencies;
  const closeMenu = () => setIsOpen(false);
  const navigate = useNavigate();
  const latencies = proxyContextValue.proxyLatencies;
  const isLoadingLatencies = Object.keys(latencies).length === 0;
  const isLoading = proxyContextValue.isLoading || isLoadingLatencies;
  const { permissions } = useAuthenticated();

  const proxyLatencyLoading = (proxy: TypesGen.Region): boolean => {
    if (!refetchDate) {
      // Only show loading if the user manually requested a refetch
      return false;
    }

    // Only show a loading spinner if:
    //  - A latency exists. This means the latency was fetched at some point, so
    //    the loader *should* be resolved.
    //  - The proxy is healthy. If it is not, the loader might never resolve.
    //  - The latency reported is older than the refetch date. This means the
    //    latency is stale and we should show a loading spinner until the new
    //    latency is fetched.
    const latency = latencies[proxy.id];
    return proxy.healthy && latency !== undefined && latency.at < refetchDate;
  };

  // This endpoint returns a 404 when not using enterprise.
  // If we don't return null, then it looks like this is
  // loading forever!
  if (proxyContextValue.error) {
    return null;
  }

  if (isLoading) {
    return (
      <Skeleton
        width="110px"
        height={BUTTON_SM_HEIGHT}
        css={{ borderRadius: 6, transform: "none" }}
      />
    );
  }

  return (
    <>
      <Button
        ref={buttonRef}
        onClick={() => setIsOpen(true)}
        size="small"
        endIcon={<KeyboardArrowDownOutlined />}
        css={{
          "& .MuiSvgIcon-root": { fontSize: 14 },
        }}
      >
        <span css={{ ...visuallyHidden }}>
          Latency for {selectedProxy?.display_name ?? "your region"}
        </span>

        {selectedProxy ? (
          <div css={{ display: "flex", gap: 8, alignItems: "center" }}>
            <div css={{ width: 16, height: 16, lineHeight: 0 }}>
              <img
                // Empty alt text used because we don't want to double up on
                // screen reader announcements from visually-hidden span
                alt=""
                src={selectedProxy.icon_url}
                css={{
                  objectFit: "contain",
                  width: "100%",
                  height: "100%",
                }}
              />
            </div>

            <Latency
              latency={latencies?.[selectedProxy.id]?.latencyMS}
              isLoading={proxyLatencyLoading(selectedProxy)}
            />
          </div>
        ) : (
          "Select Proxy"
        )}
      </Button>

      <Menu
        open={isOpen}
        anchorEl={buttonRef.current}
        onClick={closeMenu}
        onClose={closeMenu}
        css={{ "& .MuiMenu-paper": { paddingTop: 8, paddingBottom: 8 } }}
        // autoFocus here does not affect modal focus; it affects whether the
        // first item in the list will get auto-focus when the menu opens. Have
        // to turn this off because otherwise, screen readers will skip over all
        // the descriptive text and will only have access to the latency options
        autoFocus={false}
      >
        {proxyContextValue.proxies && proxyContextValue.proxies.length > 1 && (
          <>
            <div
              css={{
                width: "100%",
                maxWidth: "320px",
                fontSize: 14,
                padding: 16,
                lineHeight: "140%",
              }}
            >
              <h4
                autoFocus
                tabIndex={-1}
                css={{
                  fontSize: "inherit",
                  fontWeight: 600,
                  lineHeight: "inherit",
                  margin: 0,
                  marginBottom: 4,
                }}
              >
                Select a region nearest to you
              </h4>

              <p
                css={{
                  fontSize: 13,
                  color: theme.palette.text.secondary,
                  lineHeight: "inherit",
                  marginTop: 0.5,
                }}
              >
                Workspace proxies improve terminal and web app connections to
                workspaces. This does not apply to{" "}
                <Abbr title="Command-Line Interface" pronunciation="initialism">
                  CLI
                </Abbr>{" "}
                connections. A region must be manually selected, otherwise the
                default primary region will be used.
              </p>
            </div>

            <Divider />
          </>
        )}

        {proxyContextValue.proxies &&
          [...proxyContextValue.proxies]
            .sort((a, b) => {
              const latencyA = latencies?.[a.id]?.latencyMS ?? Infinity;
              const latencyB = latencies?.[b.id]?.latencyMS ?? Infinity;
              return latencyA - latencyB;
            })
            .map((proxy) => (
              <MenuItem
                key={proxy.id}
                selected={proxy.id === selectedProxy?.id}
                css={{ fontSize: 14 }}
                onClick={() => {
                  if (!proxy.healthy) {
                    displayError("Please select a healthy workspace proxy.");
                    closeMenu();
                    return;
                  }

                  proxyContextValue.setProxy(proxy);
                  closeMenu();
                }}
              >
                <div
                  css={{
                    display: "flex",
                    gap: 24,
                    alignItems: "center",
                    width: "100%",
                  }}
                >
                  <div css={{ width: 14, height: 14, lineHeight: 0 }}>
                    <img
                      src={proxy.icon_url}
                      alt=""
                      css={{
                        objectFit: "contain",
                        width: "100%",
                        height: "100%",
                      }}
                    />
                  </div>

                  {proxy.display_name}

                  <Latency
                    latency={latencies?.[proxy.id]?.latencyMS}
                    isLoading={proxyLatencyLoading(proxy)}
                  />
                </div>
              </MenuItem>
            ))}

        <Divider />

        {Boolean(permissions.editWorkspaceProxies) && (
          <MenuItem
            css={{ fontSize: 14 }}
            onClick={() => {
              navigate("/deployment/workspace-proxies");
            }}
          >
            Proxy settings
          </MenuItem>
        )}

        <MenuItem
          css={{ fontSize: 14 }}
          onClick={(e) => {
            // Stop the menu from closing
            e.stopPropagation();
            // Refresh the latencies.
            const refetchDate = refreshLatencies();
            setRefetchDate(refetchDate);
          }}
        >
          Refresh Latencies
        </MenuItem>
      </Menu>
    </>
  );
};
