import { Container } from '@mui/material';
import { ThemeProvider, StyledEngineProvider } from '@mui/material/styles';

import { Header } from './shared/Header';
import { Routing } from './Routing';
import { theme } from './theme';
import { HashRouter } from 'react-router-dom';
import { Attribution } from './shared/Attribution';

import { withAITracking, ReactPlugin, AppInsightsContext, AppInsightsErrorBoundary } from '@microsoft/applicationinsights-react-js';
import { ApplicationInsights } from "@microsoft/applicationinsights-web";
import { createBrowserHistory } from "history";

declare module '@mui/styles/defaultTheme' {
  // eslint-disable-next-line @typescript-eslint/no-empty-interface
  interface DefaultTheme extends Theme {}
}

var reactPlugin = new ReactPlugin();
//if (typeof window !== "undefined") {
	const browserHistory = createBrowserHistory();
	const ai = new ApplicationInsights({
    	config: {
       		instrumentationKey: '65a9a116-b453-4feb-8b8c-58efedd18626',
		extensions: [reactPlugin],
		extensionConfig: {
			[reactPlugin.identifier]: { history: browserHistory },
		},
			autoTrackPageVisitTime: true,
			enableCorsCorrelation: true,
			enableRequestHeaderTracking: true,
			enableResponseHeaderTracking: true,
			enableDebug: true,
			enableDebugExceptions: true,
    		}
	});

	ai.loadAppInsights();
	ai.trackPageView();
//}

export const appInsights = ai.appInsights
export { reactPlugin };
const App = () => {
//const App extends React.Component {
  // TODO: Removed in future version.
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.getRegistrations().then((registrations) => {
      for (const registration of registrations) {
        registration.unregister();
      }
    });
  }
  return (
<AppInsightsErrorBoundary
onError={() => (
				<h1>I believe something went wrong. Please Refresh the page!</h1>
			)}
			appInsights={reactPlugin}>
<AppInsightsContext.Provider value={reactPlugin}> 
   <StyledEngineProvider injectFirst>
      <ThemeProvider theme={theme}>
        <HashRouter>
          <Header />
          <Container maxWidth={'lg'}>
            <Routing />
	     <Attribution />
          </Container>
        </HashRouter>
      </ThemeProvider>
    </StyledEngineProvider>
</AppInsightsContext.Provider>
</AppInsightsErrorBoundary>
  );
};

export default withAITracking(reactPlugin, App);
export async function logEvent(eventName: string, properties: Object) {
    appInsights.trackEvent({ name: eventName, properties: properties });
}
