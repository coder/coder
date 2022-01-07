import React, { useState } from "react"

import { useConfetti } from "./hook"

export const Confetti: React.FC<React.HTMLAttributes<HTMLDivElement>> = (props) => {
  const [element, setElement] = useState(null)
  const getRef = (element: HTMLDivElement) => setElement(element)

  useConfetti(element)

  return <div ref={getRef} {...props} />
}
