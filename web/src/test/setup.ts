// jsdom does not implement scrolling, and the router scrolls to the top
// after every navigation; without this each one logs "Not implemented".
globalThis.scrollTo = () => undefined
