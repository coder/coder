import { type FC } from "react";

/**
 * SectionAction is a content box that call to actions should be placed
 * within
 */
export const SectionAction: FC<React.PropsWithChildren<unknown>> = ({
  children,
}) => {
  return (
    <div
      css={(theme) => ({
        marginTop: theme.spacing(3),
      })}
    >
      {children}
    </div>
  );
};
