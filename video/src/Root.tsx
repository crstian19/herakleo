import React from "react";
import { Composition } from "remotion";
import { FPS, HEIGHT, TOTAL, WIDTH } from "./timing";
import { Video } from "./Video";

export const Root: React.FC = () => (
  <Composition id="HerakleoLaunch" component={Video} durationInFrames={TOTAL} fps={FPS} width={WIDTH} height={HEIGHT} />
);
