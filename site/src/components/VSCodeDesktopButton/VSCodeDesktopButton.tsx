import React, { FC, PropsWithChildren, useState, useEffect } from "react"
import { getApiKey } from "api/api"
import { VSCodeIcon } from "components/Icons/VSCodeIcon"
import { VSCodeInsidersIcon } from "components/Icons/VSCodeInsidersIcon"
import { PrimaryAgentButton } from "components/Resources/AgentButton"
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown"

export interface VSCodeDesktopButtonProps {
  userName: string
  workspaceName: string
  agentName?: string
  folderPath?: string
}

enum VSCodeVariant {
  VSCode = "VSCode",
  VSCodeInsiders = "VSCode Insiders",
}

const getSelectedVariantFromLocalStorage = (): VSCodeVariant | null => {
  const storedVariant = localStorage.getItem("selectedVariant")
  if (
    storedVariant &&
    Object.values(VSCodeVariant).includes(storedVariant as VSCodeVariant)
  ) {
    return storedVariant as VSCodeVariant
  }
  return null
}

export const VSCodeDesktopButton: FC<
  PropsWithChildren<VSCodeDesktopButtonProps>
> = ({ userName, workspaceName, agentName, folderPath }) => {
  const [loading, setLoading] = useState(false)
  const [selectedVariant, setSelectedVariant] = useState<VSCodeVariant | null>(
    getSelectedVariantFromLocalStorage(),
  )
  const [dropdownOpen, setDropdownOpen] = useState(false)

  useEffect(() => {
    if (selectedVariant) {
      localStorage.setItem("selectedVariant", selectedVariant)
    } else {
      localStorage.removeItem("selectedVariant")
    }
  }, [selectedVariant])

  const handleButtonClick = () => {
    setLoading(true)
    getApiKey()
      .then(({ key }) => {
        const query = new URLSearchParams({
          owner: userName,
          workspace: workspaceName,
          url: location.origin,
          token: key,
        })
        if (agentName) {
          query.set("agent", agentName)
        }
        if (folderPath) {
          query.set("folder", folderPath)
        }

        const vscodeCommand =
          selectedVariant === VSCodeVariant.VSCode
            ? "vscode://"
            : "vscode-insiders://"

        location.href = `${vscodeCommand}coder.coder-remote/open?${query.toString()}`
      })
      .catch((ex) => {
        console.error(ex)
      })
      .finally(() => {
        setLoading(false)
      })
  }

  const handleVariantChange = (variant: VSCodeVariant) => {
    setSelectedVariant(variant)
    setDropdownOpen(false)
  }

  return (
    <div style={{ position: "relative", display: "inline-flex" }}>
      <PrimaryAgentButton
        startIcon={
          selectedVariant === VSCodeVariant.VSCode ? (
            <VSCodeIcon />
          ) : (
            <VSCodeInsidersIcon />
          )
        }
        disabled={loading || dropdownOpen}
        onClick={handleButtonClick}
      >
        {selectedVariant === VSCodeVariant.VSCode
          ? "VS Code Desktop"
          : "VS Code Insiders"}
      </PrimaryAgentButton>
      <PrimaryAgentButton onClick={() => setDropdownOpen(!dropdownOpen)}>
        <KeyboardArrowDownIcon
          style={{
            transition: "transform 0.3s ease-in-out",
            transform: dropdownOpen ? "rotate(180deg)" : "rotate(0)",
            cursor: "pointer",
          }}
        />
      </PrimaryAgentButton>
      {dropdownOpen && (
        <div
          style={{
            position: "absolute",
            top: "100%",
            left: 0,
            marginTop: "4px",
          }}
        >
          <PrimaryAgentButton
            onClick={() => handleVariantChange(VSCodeVariant.VSCode)}
            startIcon={<VSCodeIcon />}
          >
            VS Code Desktop
          </PrimaryAgentButton>
          <PrimaryAgentButton
            onClick={() => handleVariantChange(VSCodeVariant.VSCodeInsiders)}
            startIcon={<VSCodeInsidersIcon />}
          >
            VS Code Insiders
          </PrimaryAgentButton>
        </div>
      )}
    </div>
  )
}
