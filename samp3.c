#include<stdio.h>
 int squ(int x)
{  int sq;
  sq= x*x;
return sq;
}
 int main()
{
 int n;
  printf("enter the number :\n");
 scanf("%d",&n);
printf(" squ of %d =%d\n",n,squ(n));

}

