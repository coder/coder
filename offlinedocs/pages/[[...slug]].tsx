import {
  Box,
  Button,
  Code,
  Drawer,
  DrawerBody,
  DrawerCloseButton,
  DrawerContent,
  DrawerOverlay,
  Flex,
  Grid,
  GridProps,
  Heading,
  Icon,
  Img,
  Link,
  OrderedList,
  Table,
  TableContainer,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  UnorderedList,
  useDisclosure,
} from "@chakra-ui/react";
import fm from "front-matter";
import { readFileSync } from "fs";
import _ from "lodash";
import { GetStaticPaths, GetStaticProps, NextPage } from "next";
import Head from "next/head";
import NextLink from "next/link";
import { useRouter } from "next/router";
import path from "path";
import { MdMenu } from "react-icons/md";
import ReactMarkdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import remarkGfm from "remark-gfm";

type FilePath = string;
type UrlPath = string;
type Route = {
  path: FilePath;
  title: string;
  description?: string;
  children?: Route[];
};
type Manifest = { versions: string[]; routes: Route[] };
type NavItem = { title: string; path: UrlPath; children?: NavItem[] };
type Nav = NavItem[];

const readContentFile = (filePath: string) => {
  const baseDir = process.cwd();
  const docsPath = path.join(baseDir, "..", "docs");
  return readFileSync(path.join(docsPath, filePath), { encoding: "utf-8" });
};

const removeTrailingSlash = (path: string) => path.replace(/\/+$/, "");

const removeMkdExtension = (path: string) => path.replace(/\.md/g, "");

const removeIndexFilename = (path: string) => {
  if (path.endsWith("index")) {
    path = path.replace("index", "");
  }

  return path;
};

const removeREADMEName = (path: string) => {
  if (path.startsWith("README")) {
    path = path.replace("README", "");
  }

  return path;
};

// transformLinkUri converts the links in the markdown file to
// href html links. All index page routes are the directory name, and all
// other routes are the filename without the .md extension.
// This means all relative links are off by one directory on non-index pages.
//
// index.md -> ./subdir/file = ./subdir/file
// index.md -> ../file-next-to-index = ./file-next-to-index
// file.md -> ./subdir/file = ../subdir/file
// file.md -> ../file-next-to-file = ../file-next-to-file
const transformLinkUriSource = (sourceFile: string) => {
  return (href = "") => {
    const isExternal = href.startsWith("http") || href.startsWith("https");
    if (!isExternal) {
      // Remove .md form the path
      href = removeMkdExtension(href);

      // Add the extra '..' if not an index file.
      sourceFile = removeMkdExtension(sourceFile);
      if (!sourceFile.endsWith("index")) {
        href = "../" + href;
      }

      // Remove the index path
      href = removeIndexFilename(href);
      href = removeREADMEName(href);
    }
    return href;
  };
};

const transformFilePathToUrlPath = (filePath: string) => {
  // Remove markdown extension
  let urlPath = removeMkdExtension(filePath);

  // Remove relative path
  if (urlPath.startsWith("./")) {
    urlPath = urlPath.replace("./", "");
  }

  // Remove index from the root file
  urlPath = removeIndexFilename(urlPath);
  urlPath = removeREADMEName(urlPath);

  // Remove trailing slash
  if (urlPath.endsWith("/")) {
    urlPath = removeTrailingSlash(urlPath);
  }

  return urlPath;
};

const mapRoutes = (manifest: Manifest): Record<UrlPath, Route> => {
  const paths: Record<UrlPath, Route> = {};

  const addPaths = (routes: Route[]) => {
    for (const route of routes) {
      paths[transformFilePathToUrlPath(route.path)] = route;

      if (route.children) {
        addPaths(route.children);
      }
    }
  };

  addPaths(manifest.routes);

  return paths;
};

let manifest: Manifest | undefined;

const getManifest = () => {
  if (manifest) {
    return manifest;
  }

  const manifestContent = readContentFile("manifest.json");
  manifest = JSON.parse(manifestContent) as Manifest;
  return manifest;
};

let navigation: Nav | undefined;

const getNavigation = (manifest: Manifest): Nav => {
  if (navigation) {
    return navigation;
  }

  const getNavItem = (route: Route, parentPath?: UrlPath): NavItem => {
    const path = parentPath
      ? `${parentPath}/${transformFilePathToUrlPath(route.path)}`
      : transformFilePathToUrlPath(route.path);
    const navItem: NavItem = {
      title: route.title,
      path,
    };

    if (route.children) {
      navItem.children = [];

      for (const childRoute of route.children) {
        navItem.children.push(getNavItem(childRoute));
      }
    }

    return navItem;
  };

  navigation = [];

  for (const route of manifest.routes) {
    navigation.push(getNavItem(route));
  }

  return navigation;
};

const removeHtmlComments = (string: string) => {
  return string.replace(/<!--[\s\S]*?-->/g, "");
};

export const getStaticPaths: GetStaticPaths = () => {
  const manifest = getManifest();
  const routes = mapRoutes(manifest);
  const paths = Object.keys(routes).map((urlPath) => ({
    params: { slug: urlPath.split("/") },
  }));

  return {
    paths,
    fallback: false,
  };
};

export const getStaticProps: GetStaticProps = (context) => {
  // When it is home page, the slug is undefined because there is no url path
  // so we make it an empty string to work good with the mapRoutes
  const { slug = [""] } = context.params as { slug: string[] };
  const manifest = getManifest();
  const routes = mapRoutes(manifest);
  const urlPath = slug.join("/");
  const route = routes[urlPath];
  const { body } = fm(readContentFile(route.path));
  // Serialize MDX to support custom components
  const content = removeHtmlComments(body);
  const navigation = getNavigation(manifest);
  const version = manifest.versions[0];

  return {
    props: {
      content,
      navigation,
      route,
      version,
    },
  };
};

const SidebarNavItem: React.FC<{ item: NavItem; nav: Nav }> = ({
  item,
  nav,
}) => {
  const router = useRouter();
  let isActive = router.asPath.startsWith(`/${item.path}`);

  // Special case to handle the home path
  if (item.path === "") {
    isActive = router.asPath === "/";

    // Special case to handle the home path children
    const homeNav = nav.find((navItem) => navItem.path === "") as NavItem;
    const homeNavPaths =
      homeNav.children?.map((item) => `/${item.path}/`) ?? [];
    if (homeNavPaths.includes(router.asPath)) {
      isActive = true;
    }
  }

  return (
    <Box>
      <NextLink href={"/" + item.path} passHref>
        <Link
          fontWeight={isActive ? 600 : 400}
          color={isActive ? "gray.900" : "gray.700"}
        >
          {item.title}
        </Link>
      </NextLink>

      {isActive && item.children && (
        <Grid
          as="nav"
          pt={2}
          pl={3}
          maxW="sm"
          autoFlow="row"
          gap={2}
          autoRows="min-content"
        >
          {item.children.map((subItem) => (
            <SidebarNavItem key={subItem.path} item={subItem} nav={nav} />
          ))}
        </Grid>
      )}
    </Box>
  );
};

const SidebarNav: React.FC<{ nav: Nav; version: string } & GridProps> = ({
  nav,
  version,
  ...gridProps
}) => {
  return (
    <Grid
      h="100vh"
      overflowY="scroll"
      as="nav"
      p={8}
      w="300px"
      autoFlow="row"
      gap={2}
      autoRows="min-content"
      bgColor="white"
      borderRightWidth={1}
      borderColor="gray.200"
      borderStyle="solid"
      {...gridProps}
    >
      <Box mb={6}>
        <Img src="/logo.svg" alt="Coder logo" />
      </Box>

      {nav.map((navItem) => (
        <SidebarNavItem key={navItem.path} item={navItem} nav={nav} />
      ))}
    </Grid>
  );
};

const MobileNavbar: React.FC<{ nav: Nav; version: string }> = ({
  nav,
  version,
}) => {
  const { isOpen, onOpen, onClose } = useDisclosure();

  return (
    <>
      <Flex
        bgColor="white"
        px={6}
        alignItems="center"
        h={16}
        borderBottomWidth={1}
      >
        <Img src="/logo.svg" alt="Coder logo" w={28} />

        <Button variant="ghost" ml="auto" onClick={onOpen}>
          <Icon as={MdMenu} fontSize="2xl" />
        </Button>
      </Flex>

      <Drawer onClose={onClose} isOpen={isOpen}>
        <DrawerOverlay />
        <DrawerContent>
          <DrawerCloseButton />
          <DrawerBody p={0}>
            <SidebarNav nav={nav} version={version} border={0} />
          </DrawerBody>
        </DrawerContent>
      </Drawer>
    </>
  );
};

const slugifyTitle = (title: string) => {
  return _.kebabCase(title.toLowerCase());
};

const getImageUrl = (src: string | undefined) => {
  if (src === undefined) {
    return "";
  }
  const assetPath = src.split("images/")[1];
  return `/images/${assetPath}`;
};

const DocsPage: NextPage<{
  content: string;
  navigation: Nav;
  route: Route;
  version: string;
}> = ({ content, navigation, route, version }) => {
  return (
    <>
      <Head>
        <title>{route.title}</title>
        <meta name="source" content={route.path} />
      </Head>
      <Box
        display={{ md: "grid" }}
        gridTemplateColumns="max-content 1fr"
        fontSize="md"
        color="gray.700"
      >
        <Box display={{ base: "none", md: "block" }}>
          <SidebarNav nav={navigation} version={version} />
        </Box>

        <Box display={{ base: "block", md: "none" }}>
          <MobileNavbar nav={navigation} version={version} />
        </Box>

        <Box
          as="main"
          w="full"
          pb={20}
          px={{ base: 6, md: 10 }}
          pl={{ base: 6, md: 20 }}
          h="100vh"
          overflowY="auto"
        >
          <Box maxW="872">
            <Box lineHeight="tall">
              {/* Some docs don't have the title */}
              <Heading
                as="h1"
                fontSize="4xl"
                pt={10}
                pb={2}
                // Hide this title if the doc has the title already
                sx={{ "& + h1": { display: "none" } }}
              >
                {route.title}
              </Heading>
              <ReactMarkdown
                rehypePlugins={[rehypeRaw]}
                remarkPlugins={[remarkGfm]}
                transformLinkUri={transformLinkUriSource(route.path)}
                components={{
                  h1: ({ children }) => (
                    <Heading
                      as="h1"
                      fontSize="4xl"
                      pt={10}
                      pb={2}
                      id={slugifyTitle(children[0] as string)}
                    >
                      {children}
                    </Heading>
                  ),
                  h2: ({ children }) => (
                    <Heading
                      as="h2"
                      fontSize="3xl"
                      pt={10}
                      pb={2}
                      id={slugifyTitle(children[0] as string)}
                    >
                      {children}
                    </Heading>
                  ),
                  h3: ({ children }) => (
                    <Heading
                      as="h3"
                      fontSize="2xl"
                      pt={10}
                      pb={2}
                      id={slugifyTitle(children[0] as string)}
                    >
                      {children}
                    </Heading>
                  ),
                  img: ({ src }) => (
                    <Img
                      src={getImageUrl(src)}
                      mb={2}
                      borderWidth={1}
                      borderColor="gray.200"
                      borderStyle="solid"
                      rounded="md"
                      height="auto"
                    />
                  ),
                  p: ({ children }) => (
                    <Text pt={2} pb={2}>
                      {children}
                    </Text>
                  ),
                  ul: ({ children }) => (
                    <UnorderedList
                      mb={4}
                      display="grid"
                      gridAutoFlow="row"
                      gap={2}
                    >
                      {children}
                    </UnorderedList>
                  ),
                  ol: ({ children }) => (
                    <OrderedList
                      mb={4}
                      display="grid"
                      gridAutoFlow="row"
                      gap={2}
                    >
                      {children}
                    </OrderedList>
                  ),
                  a: ({ children, href = "" }) => {
                    const isExternal =
                      href.startsWith("http") || href.startsWith("https");

                    return (
                      <Link
                        href={href}
                        target={isExternal ? "_blank" : undefined}
                        fontWeight={500}
                        color="blue.600"
                      >
                        {children}
                      </Link>
                    );
                  },
                  code: ({ node, ...props }) => (
                    <Code {...props} bgColor="gray.100" />
                  ),
                  pre: ({ children }) => (
                    <Box
                      as="pre"
                      w="full"
                      sx={{ "& > code": { w: "full", p: 4, rounded: "md" } }}
                      mb={2}
                    >
                      {children}
                    </Box>
                  ),
                  table: ({ children }) => (
                    <TableContainer
                      mt={1}
                      mb={2}
                      bgColor="white"
                      rounded="md"
                      borderWidth={1}
                      borderColor="gray.100"
                      borderStyle="solid"
                    >
                      <Table variant="simple">{children}</Table>
                    </TableContainer>
                  ),
                  thead: ({ children }) => <Thead>{children}</Thead>,
                  th: ({ children }) => <Th>{children}</Th>,
                  td: ({ children }) => <Td>{children}</Td>,
                  tr: ({ children }) => <Tr>{children}</Tr>,
                }}
              >
                {content}
              </ReactMarkdown>
            </Box>
          </Box>
        </Box>
      </Box>
    </>
  );
};

export default DocsPage;
