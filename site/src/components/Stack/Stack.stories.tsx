import type { Meta, StoryObj } from "@storybook/react";
import { Stack } from "./Stack";

const meta: Meta<typeof Stack> = {
  title: "components/Stack",
  component: Stack,
  args: {
    children: (
      <>
        <span>チェンソーマン</span>
        <span>ジョジョの奇妙な冒険</span>
        <span>スパイファミリー</span>
        <span>葬送のフリーレン</span>
        <span>少女革命ウテナ</span>
        <span>PSYCHO-PASS サイコパス</span>
        <span>機動戦士ガンダム 水星の魔女</span>
        <span>勇気爆発バーンブレイバーン</span>
        <span>Re:ゼロから始める異世界生活</span>
        <span>ダンジョン飯</span>
      </>
    ),
  },
};

export default meta;
type Story = StoryObj<typeof Stack>;

export const Vertical: Story = {};

export const VerticalCenter: Story = {
  args: {
    alignItems: "center",
  },
};

export const Horizontal: Story = {
  args: {
    direction: "row",
  },
};

export const HorizontalWrap: Story = {
  args: {
    direction: "row",
    wrap: "wrap",
  },
};
