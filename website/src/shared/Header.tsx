<<<<<<< Updated upstream
import { AppBar, Toolbar, Typography, Button, Box, Link } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router-dom';
import makeStyles from '@mui/styles/makeStyles';

export const Header = () => {
  const base = process.env.PUBLIC_URL || '';
  const home = base + '/#/';
  #const upload = base + '/#/upload';
  const classes = useStyles();
  return (
    <AppBar position="static" color="transparent" sx={{ marginBottom: 4 }}>
      <Toolbar>
        <Link href={home} color="inherit" underline="none">
          <Typography variant="h6" component="div">
            Cred
            <img
              className={classes.logo}
              width="40"
              height="40"
              alt=""
              src="cred.svg"
            />
          </Typography>
        </Link>
        <Box
          sx={{
            marginLeft: 'auto',
          }}
        >
        </Box>
      </Toolbar>
    </AppBar>
  );
};
