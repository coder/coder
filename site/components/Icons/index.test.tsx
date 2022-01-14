import React from "react"
import { SvgIcon } from "@material-ui/core"
import { render } from "./../../test_helpers"

import * as Icons from "./index"

const getAllIcons = (): [string, typeof SvgIcon][] => {
  let k: keyof typeof Icons
  let ret: [string, typeof SvgIcon][] = []
  for (k in Icons) {
    ret.push([k, Icons[k]])
  }
  return ret
}

describe("Icons", () => {
  const allIcons = getAllIcons()

  it.each(allIcons)(`rendering icon %p`, (_name, Icon) => {
    render(<Icon />)
  })
})
