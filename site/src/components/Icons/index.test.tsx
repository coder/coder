import SvgIcon from "@material-ui/core/SvgIcon"
import React from "react"
import { render } from "../../testHelpers"
import * as Icons from "./index"

const getAllIcons = (): [string, typeof SvgIcon][] => {
  let k: keyof typeof Icons
  const ret: [string, typeof SvgIcon][] = []
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
