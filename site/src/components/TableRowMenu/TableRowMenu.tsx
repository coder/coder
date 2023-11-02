import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";

export interface TableRowMenuProps<TData> {
  data: TData;
  menuItems: Array<{
    label: React.ReactNode;
    disabled: boolean;
    onClick: (data: TData) => void;
  }>;
}

export const TableRowMenu = <T,>({
  data,
  menuItems,
}: TableRowMenuProps<T>): JSX.Element => {
  return (
    <MoreMenu>
      <MoreMenuTrigger aria-label="more" aria-controls="long-menu" />
      <MoreMenuContent id="simple-menu" keepMounted>
        {menuItems.map((item, index) => (
          <MoreMenuItem
            key={index}
            disabled={item.disabled}
            onClick={() => {
              item.onClick(data);
            }}
          >
            {item.label}
          </MoreMenuItem>
        ))}
      </MoreMenuContent>
    </MoreMenu>
  );
};
