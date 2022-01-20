/**
 * Returns a promise that resolves after a set time
 *
 * @param time Time to wait in milliseconds
 */
export async function wait(milliseconds: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, milliseconds))
}
