
import { ConfettiTransition } from "../m/product/coder/site/src/pages/Onboarding/Activation/ConfettiTransition"
import { decrementWithFloor } from "lib/ts/x/decrement-with-floor"
function component() {
  const element = document.createElement('div');

  element.innerHTML = "Hello Webpack"

  console.log(decrementWithFloor(1.0, 1.0))

  console.log(ConfettiTransition.toString)

  return element;
}

document.body.appendChild(component());
