pipeline
{
   agent none

   options
   {
      buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '10', daysToKeepStr: '30', numToKeepStr: '10')
      timestamps()
      withAWS(region:'eu-west-1', credentials:'aws-key-releases')
   }

   tools
   {
     go 'Go'
   }

   environment
   {
     GOPATH = '/home/jenkins/go'
     GO15VENDOREXPERIMENT=1
   }

   stages
   {
      stage('Build')
      {
          agent
          {
            label 'linux'
          }
          steps
          {
             sh 'go version'
             sh 'go mod vendor'
             sh "make test"
             sh 'make build-all'
             stash name: 'build artifacts', includes: 'build/**'
          }

          post
          {
            cleanup
            {
              cleanWs()
            }
          }
       }

      stage('Package')
      {
         agent
         {
           docker
           {
             label 'linux'
             image 'torch/jenkins-fpm-cook:latest'
             args '-u jenkins:docker'
           }
         }

         steps
         {
            unstash 'build artifacts'
            sh 'make package-all'

            stash name: 'package artifacts', includes: 'dist/pkg/**'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
            }

            cleanup
            {
              cleanWs()
            }
         }
      }

      stage('Release to Package Repository')
      {
         when
         {
             buildingTag()
         }

         agent
         {
           label 'packages'
         }

         steps
         {
           script
           {
             // Build release branch name from release tag name
             def (_, major, minor) = (env.TAG_NAME =~ /^(\d+)\.(\d+)\.\d+(?:-.*)?/)[0]

             env.SIDECAR_RELEASE_VERSION = "${major}.${minor}"
           }

           echo "Checking out fpm-recipes branch: ${SIDECAR_RELEASE_VERSION}"
           checkout poll: false, scm: [$class: 'GitSCM', branches: [[name: "*/${SIDECAR_RELEASE_VERSION}"]], doGenerateSubmoduleConfigurations: false, extensions: [[$class: 'WipeWorkspace']], submoduleCfg: [], userRemoteConfigs: [[credentialsId: 'github-access-token2', url: 'https://github.com/Graylog2/fpm-recipes.git']]]

           sh "gl2-build-pkg-sidecar ${SIDECAR_RELEASE_VERSION}"
         }

         post
         {
           cleanup
           {
             cleanWs()
           }
         }
      }
   }
}
