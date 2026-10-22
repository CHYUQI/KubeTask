// Keep the catalog's layout coordinates. Allow the original pressed indicator
// and backing to expand with their full shadows, omitting the second demo bar.
export const NAVIGATION_CATALOG_HEIGHT = 208;
export const NAVIGATION_FRAME_HEIGHT = 160;

export function prepareNavigationElements(elements, labels = ["总览", "任务", "工作流"], fontSize = 20) {
  return elements
    .filter((element) => element.id.startsWith("tabs3"))
    .map((element) => {
      return element.text
        ? {
            ...element,
            text: {
              ...element.text,
              content: labels[Number(element.id.split("-").at(-1))],
              fontSizePx: fontSize,
              icon: undefined,
              halo: "none",
            },
          }
        : element;
    });
}
