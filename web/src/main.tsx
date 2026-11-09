import "@fontsource/chakra-petch/600.css";
import "@fontsource/chakra-petch/700.css";
import "@fontsource/montserrat/400.css";
import "@fontsource/montserrat/500.css";
import "@fontsource/montserrat/600.css";
import "@fontsource/jetbrains-mono/400.css";
import "@fontsource-variable/inter";
import "./styles.css";
import { render } from "preact";
import { App } from "./ui/App";

render(<App />, document.getElementById("root")!);
