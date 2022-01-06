import React from "react"
import ReactDOM from "react-dom"

function component() {
  const element = document.createElement('div');
  
  ReactDOM.render(<div>Hi</div>, element)

  return element;
}

document.body.appendChild(component());
