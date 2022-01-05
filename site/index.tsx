import React from "react"
import ReactDOM from "react-dom"

// Code imported from 'Coder v1'
// Legacy components that we can re-use
import { CoderThemeProvider } from "@v1/product/coder/site/src/contexts/CoderThemeProvider"
import { UserContext, defaultAppState } from "@v1/product/coder/site/src/contexts/User"
import { SetupModeProvider } from "@v1/product/coder/site/src/contexts/SetupMode"

// A fun component to bring in as an example of re-using UI from 'v1':
import { ConfettiTransition } from "@v1/product/coder/site/src/pages/Onboarding/Activation/ConfettiTransition"

// And some other, simpler, componenents:
import { PrimaryButton } from "@v1/product/coder/site/src/components/Button"

// Helper function to scaffold providers that are needed to host the 'V1' UI
// Ultimately, we'd host the entire app (router and all)
const renderLegacyComponent = (component: JSX.Element) => {
  const fetchSetupMode = async () => {
    // Return an empty response for 'setup mode'
    return {
      body: null
    } as any;
  }

  return <UserContext.Provider value={defaultAppState}>
    <SetupModeProvider fetchSetupMode={fetchSetupMode}>
      <CoderThemeProvider>
        {component}
      </CoderThemeProvider>
    </SetupModeProvider>
  </UserContext.Provider>
}

function component() {
  const element = document.createElement('div');

  // Render the legacy UI
  const ui = renderLegacyComponent(<div>
    <ConfettiTransition />
    <div style={{ position: 'absolute', height: 250, left: 0, right: 0, bottom: 0 }}>
      <PrimaryButton>A v1 button hosted in v2 app</PrimaryButton>
    </div>
  </div >)
  ReactDOM.render(ui, element)

  return element;
}

document.body.appendChild(component());
