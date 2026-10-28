// jsdom does not implement scrolling, and the router scrolls to the top
// after every navigation; without this each one logs "Not implemented".
globalThis.scrollTo = () => undefined

// Nor does it scroll an element into view, as a field does that takes the
// focus to say why it cannot be saved.
Element.prototype.scrollIntoView = () => undefined
