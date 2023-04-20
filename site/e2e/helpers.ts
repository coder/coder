import path from "path"

const statesDir = path.join(__dirname, "./states")

export const getStatePath = (name: string): string => {
  return path.join(statesDir, `${name}.json`)
}
