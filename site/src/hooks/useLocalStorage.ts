export const useLocalStorage = () => {
  return {
    saveLocal,
    getLocal,
    clearLocal,
  };
};

const saveLocal = (itemKey: string, itemValue: string): void => {
  window.localStorage.setItem(itemKey, itemValue);
};

const getLocal = (itemKey: string): string | undefined => {
  return localStorage.getItem(itemKey) ?? undefined;
};

const clearLocal = (itemKey: string): void => {
  localStorage.removeItem(itemKey);
};
