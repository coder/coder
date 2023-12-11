'use strict'

const { safe } = require('./safe-format')

const caches = new WeakMap()

const scopeMethods = (scope) => {
  // cache meta info for known scope variables, per meta type
  if (!caches.has(scope))
    caches.set(scope, { sym: new Map(), ref: new Map(), format: new Map(), pattern: new Map() })
  const cache = caches.get(scope)

  const gensym = (name) => {
    if (!cache.sym.get(name)) cache.sym.set(name, 0)
    const index = cache.sym.get(name)
    cache.sym.set(name, index + 1)
    return safe(`${name}${index}`)
  }

  const genpattern = (p) => {
    if (cache.pattern.has(p)) return cache.pattern.get(p)
    const n = gensym('pattern')
    scope[n] = new RegExp(p, 'u')
    cache.pattern.set(p, n)
    return n
  }

  if (!cache.loop) cache.loop = 'ijklmnopqrstuvxyz'.split('')
  const genloop = () => {
    const v = cache.loop.shift()
    cache.loop.push(`${v}${v[0]}`)
    return safe(v)
  }

  const getref = (sub) => cache.ref.get(sub)
  const genref = (sub) => {
    const n = gensym('ref')
    cache.ref.set(sub, n)
    return n
  }

  const genformat = (impl) => {
    let n = cache.format.get(impl)
    if (!n) {
      n = gensym('format')
      scope[n] = impl
      cache.format.set(impl, n)
    }
    return n
  }

  return { gensym, genpattern, genloop, getref, genref, genformat }
}

module.exports = { scopeMethods }
