import * as API from "api/api";

export const experiments = () => {
  return {
    queryKey: ["experiments"],
    queryFn: fetchExperiments,
  };
};

const fetchExperiments = async () => {
  // Experiments is injected by the Coder server into the HTML document.
  const experiments = document.querySelector("meta[property=experiments]");
  if (experiments) {
    const rawContent = experiments.getAttribute("content");
    try {
      return JSON.parse(rawContent as string);
    } catch (ex) {
      // Ignore this and fetch as normal!
    }
  }

  return API.getExperiments();
};
