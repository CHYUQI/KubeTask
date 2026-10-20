export function prepareNavigationElements(elements) {
  return elements
    .filter((element) => element.id.startsWith("tabs3"))
    .map((element) => {
      if (element.isBottomTabContainer) {
        // Keep the container marker for the upstream backdrop snapshot, but
        // detach its transform/glow from the indicator's animated group.
        element.isBottomTabContainer = {
          ...element.isBottomTabContainer,
          groupId: "tabs3-background",
        };
      }
      if (element.text)
        element.text = {
          ...element.text,
          content: "",
          icon: undefined,
          halo: "none",
        };
      return element;
    });
}
