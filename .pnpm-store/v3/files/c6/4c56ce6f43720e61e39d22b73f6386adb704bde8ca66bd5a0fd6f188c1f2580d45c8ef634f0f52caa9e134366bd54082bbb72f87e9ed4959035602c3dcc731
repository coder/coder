'use strict'

module.exports = function (obj, pair) {
  if (obj[pair.name] === undefined) {
    obj[pair.name] = pair.value
    return obj
  }

  // If we already have it as array just push the value
  if (obj[pair.name] instanceof Array) {
    obj[pair.name].push(pair.value)
    return obj
  }

  // convert to array
  var arr = [
    obj[pair.name],
    pair.value
  ]

  obj[pair.name] = arr

  return obj
}
