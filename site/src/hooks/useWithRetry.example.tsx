import React from "react";
import { useWithRetry } from "./useWithRetry";

// Example component showing how to use useWithRetry
export const TerminalConnectionExample: React.FC = () => {
  // Mock terminal connection function
  const connectToTerminal = async (): Promise<void> => {
    // Simulate connection that might fail
    if (Math.random() > 0.7) {
      throw new Error("Connection failed");
    }
    console.log("Connected to terminal successfully!");
  };

  const { call: connectTerminal, isLoading, retryAt } = useWithRetry(
    connectToTerminal,
    {
      maxAttempts: 3,
      initialDelay: 1000,
      maxDelay: 5000,
      multiplier: 2,
    },
  );

  const formatRetryTime = (date: Date): string => {
    const seconds = Math.ceil((date.getTime() - Date.now()) / 1000);
    return `${seconds}s`;
  };

  return (
    <div>
      <button onClick={connectTerminal} disabled={isLoading}>
        {isLoading ? "Connecting..." : "Connect to Terminal"}
      </button>
      
      {retryAt && (
        <div>
          <p>Connection failed. Retrying in {formatRetryTime(retryAt)}</p>
        </div>
      )}
    </div>
  );
};

// Example with different configuration
export const QuickRetryExample: React.FC = () => {
  const performAction = async (): Promise<void> => {
    // Simulate an action that might fail
    throw new Error("Action failed");
  };

  const { call, isLoading, retryAt } = useWithRetry(performAction, {
    maxAttempts: 5,
    initialDelay: 500,
    multiplier: 1.5,
  });

  return (
    <div>
      <button onClick={call} disabled={isLoading}>
        {isLoading ? "Processing..." : "Perform Action"}
      </button>
      
      {retryAt && (
        <p>Retrying at {retryAt.toLocaleTimeString()}</p>
      )}
    </div>
  );
};
